//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gyebran/GoWork/internal/assets"
	"github.com/Gyebran/GoWork/internal/auditlog"
	"github.com/Gyebran/GoWork/internal/auth"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/Gyebran/GoWork/internal/users"
	"github.com/Gyebran/GoWork/internal/workorders"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type traceKey struct{}
type countBarrier struct {
	once       sync.Once
	afterCount func()
}

func (b *countBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, traceKey{}, strings.HasPrefix(d.SQL, "-- name: CountAssets"))
}
func (b *countBarrier) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if match, _ := ctx.Value(traceKey{}).(bool); match && d.Err == nil {
		b.once.Do(b.afterCount)
	}
}

func hardeningIntegration(t *testing.T, ctx context.Context, owner *pgxpool.Pool) {
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	defer exec("DELETE FROM work_orders;DELETE FROM assets;DELETE FROM audit_logs;DELETE FROM users")
	// Exactly the documented runtime grants, under a disposable role in the guarded test DB.
	exec("CREATE ROLE gowork_hardening_runtime NOLOGIN")
	defer exec("DROP OWNED BY gowork_hardening_runtime; DROP ROLE gowork_hardening_runtime")
	grants, err := os.ReadFile("../../db/local/grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(strings.ReplaceAll(string(grants), "gowork_app", "gowork_hardening_runtime"))
	inserted := make(chan error, 1)
	barrier := &countBarrier{afterCount: func() {
		_, err := owner.Exec(ctx, "INSERT INTO assets(asset_code,name,category,location) VALUES('M8-CONCURRENT','Concurrent snapshot fixture','Test','Test')")
		inserted <- err
	}}
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Tracer = barrier
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE gowork_hardening_runtime")
		return err
	}
	runtime, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	var role string
	if err = runtime.QueryRow(ctx, "SELECT current_user").Scan(&role); err != nil || role != "gowork_hardening_runtime" {
		t.Fatal("wrong effective role", err)
	}
	if err = auth.SeedDemo(ctx, runtime, "test"); err != nil {
		t.Fatal(err)
	}
	tokens, _ := auth.NewTokens("integration-signing-key-with-at-least-32-bytes", "gowork", "gowork-api")
	as, err := auth.NewService(dbsql.New(runtime), tokens)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ah := auth.NewHandler(as, logger)
	router := httpx.NewRouterWithRoutes(logger, nil, func(r chi.Router) {
		ah.Register(r)
		users.NewHandler(users.NewService(runtime), ah, logger).Register(r)
		assets.NewHandler(assets.NewService(runtime), ah, logger).Register(r)
		workorders.NewHandler(workorders.NewService(runtime), ah, logger).Register(r)
		auditlog.NewHandler(auditlog.NewService(runtime), ah, logger).Register(r)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	client.Timeout = 15 * time.Second
	call := func(t *testing.T, method, path, token, body string, want int) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "m8-network")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s got %d want %d %s", method, path, res.StatusCode, want, b)
		}
		if res.Header.Get("X-Request-ID") != "m8-network" {
			t.Fatal("request ID lost")
		}
		if want >= 400 {
			var envelope struct {
				RequestID string `json:"request_id"`
			}
			if json.Unmarshal(b, &envelope) != nil || envelope.RequestID != "m8-network" {
				t.Fatal("error correlation")
			}
		}
		return b
	}
	login := func(t *testing.T, email string) string {
		t.Helper()
		b := call(t, "POST", "/api/v1/auth/login", "", `{"email":"`+email+`","password":"GoWork-demo-only-2026!"}`, 200)
		var v struct {
			Data auth.LoginResult `json:"data"`
		}
		if json.Unmarshal(b, &v) != nil || v.Data.AccessToken == "" {
			t.Fatal("no token")
		}
		return v.Data.AccessToken
	}
	adminToken := login(t, "admin@gowork.dev")
	techToken := login(t, "technician@gowork.dev")
	assetBody := call(t, "POST", "/api/v1/assets", adminToken, `{"asset_code":"M8","name":"Network asset","category":"Test","location":"Test"}`, 201)
	var asset struct {
		Data assets.Asset `json:"data"`
	}
	if json.Unmarshal(assetBody, &asset) != nil {
		t.Fatal("asset JSON")
	}
	t.Run("runtime grants support network lifecycle and audit", func(t *testing.T) {
		call(t, "POST", "/api/v1/users", adminToken, `{"name":"bad\u0000name","email":"nul@example.com","password":"integration-password-2026!","role":"TECHNICIAN"}`, 422)
		b := call(t, "POST", "/api/v1/work-orders", adminToken, `{"asset_id":"`+asset.Data.ID+`","title":"Network order"}`, 201)
		var o struct {
			Data workorders.Order `json:"data"`
		}
		if json.Unmarshal(b, &o) != nil {
			t.Fatal("order JSON")
		}
		tech, err := dbsql.New(owner).GetLoginUser(ctx, "technician@gowork.dev")
		if err != nil {
			t.Fatal(err)
		}
		path := "/api/v1/work-orders/" + o.Data.ID
		call(t, "PATCH", path+"/assign", adminToken, `{"assigned_to":"`+tech.ID.String()+`"}`, 200)
		call(t, "PATCH", path+"/status", techToken, `{"status":"IN_PROGRESS"}`, 200)
		call(t, "PATCH", path+"/status", techToken, `{"status":"COMPLETED"}`, 200)
		call(t, "GET", "/api/v1/audit-logs", techToken, "", 403)
		b = call(t, "GET", "/api/v1/audit-logs?entity_type=work_order&entity_id="+o.Data.ID, adminToken, "", 200)
		var p auditlog.Page
		if json.Unmarshal(b, &p) != nil || p.Meta.Total != 5 {
			t.Fatal("network audit sequence", string(b))
		}
		for _, sql := range []string{"UPDATE audit_logs SET request_id='forbidden'", "DELETE FROM audit_logs", "CREATE TABLE forbidden_m8(id int)"} {
			if _, err := runtime.Exec(ctx, sql); err == nil {
				t.Fatal("runtime privilege leak", sql)
			}
		}
	})
	t.Run("list and count remain consistent across committed insert", func(t *testing.T) {
		b := call(t, "GET", "/api/v1/assets", adminToken, "", 200)
		var p assets.Page
		if json.Unmarshal(b, &p) != nil {
			t.Fatal("page JSON")
		}
		select {
		case err := <-inserted:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("count barrier not reached")
		}
		if p.Meta.Total != 1 || len(p.Data) != 1 || p.Data[0].AssetCode != "M8" {
			t.Fatal("mixed list/count snapshots", string(b))
		}
		b = call(t, "GET", "/api/v1/assets", adminToken, "", 200)
		if json.Unmarshal(b, &p) != nil || p.Meta.Total != 2 || len(p.Data) != 2 {
			t.Fatal("new request did not see committed row", string(b))
		}
	})
	t.Run("unauthorized writes leave business state and audit unchanged", func(t *testing.T) {
		var before, after int
		if err := owner.QueryRow(ctx, "SELECT count(*) FROM audit_logs").Scan(&before); err != nil {
			t.Fatal(err)
		}
		call(t, "PATCH", "/api/v1/assets/"+asset.Data.ID, techToken, `{"name":"Unauthorized mutation"}`, 403)
		call(t, "DELETE", "/api/v1/assets/"+asset.Data.ID, techToken, "", 403)
		if err := owner.QueryRow(ctx, "SELECT count(*) FROM audit_logs").Scan(&after); err != nil || after != before {
			t.Fatal("denial wrote audit", err)
		}
		var name string
		if err := owner.QueryRow(ctx, "SELECT name FROM assets WHERE id=$1", asset.Data.ID).Scan(&name); err != nil || name != "Network asset" {
			t.Fatal("denial changed row", err)
		}
	})
}
