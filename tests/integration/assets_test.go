//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Gyebran/GoWork/internal/assets"
	"github.com/Gyebran/GoWork/internal/auth"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func assetsIntegration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	defer exec("DELETE FROM work_orders;DELETE FROM assets;DELETE FROM audit_logs;DELETE FROM users")
	if err := auth.SeedDemo(ctx, pool, "test"); err != nil {
		t.Fatal(err)
	}
	q := dbsql.New(pool)
	admin, err := q.GetLoginUser(ctx, "admin@gowork.dev")
	if err != nil {
		t.Fatal(err)
	}
	tech, err := q.GetLoginUser(ctx, "technician@gowork.dev")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := q.GetLoginUser(ctx, "manager@gowork.dev")
	if err != nil {
		t.Fatal(err)
	}
	tokens, _ := auth.NewTokens("integration-signing-key-with-at-least-32-bytes", "gowork", "gowork-api")
	as, err := auth.NewService(q, tokens)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ah := auth.NewHandler(as, logger)
	svc := assets.NewService(pool)
	h := assets.NewHandler(svc, ah, logger)
	router := httpx.NewRouterWithRoutes(logger, nil, func(r chi.Router) { ah.Register(r); h.Register(r) })
	at, _ := tokens.Issue(admin.ID.String())
	mt, _ := tokens.Issue(manager.ID.String())
	tt, _ := tokens.Issue(tech.ID.String())
	call := func(t *testing.T, method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "m5-test")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	code := func(t *testing.T, w *httptest.ResponseRecorder, want string) {
		t.Helper()
		if !strings.Contains(w.Body.String(), `"code":"`+want+`"`) {
			t.Fatal(w.Body.String())
		}
	}
	decode := func(t *testing.T, w *httptest.ResponseRecorder) assets.Asset {
		t.Helper()
		var d struct {
			Data assets.Asset `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d.Data
	}
	create := func(t *testing.T, c, name string) assets.Asset {
		t.Helper()
		b, _ := json.Marshal(map[string]string{"asset_code": c, "name": name, "category": "HVAC", "location": "Room A"})
		w := call(t, "POST", "/api/v1/assets", mt, string(b), 201)
		a := decode(t, w)
		if w.Header().Get("Location") != "/api/v1/assets/"+a.ID || a.Status != "ACTIVE" {
			t.Fatal("create contract")
		}
		return a
	}
	list := func(t *testing.T, token, query string) assets.Page {
		t.Helper()
		w := call(t, "GET", "/api/v1/assets"+query, token, "", 200)
		var p assets.Page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := create(t, "M5-A", "Pump 100%_\\ test")
	b := create(t, "M5-B", "Other pump")
	aid, _ := assets.ID(a.ID)
	t.Run("permissions validation and immutable code", func(t *testing.T) {
		call(t, "GET", "/api/v1/assets", "", "", 401)
		call(t, "POST", "/api/v1/assets", tt, `{}`, 403)
		call(t, "PATCH", "/api/v1/assets/"+a.ID, tt, `{"name":"no"}`, 403)
		call(t, "DELETE", "/api/v1/assets/"+a.ID, mt, "", 403)
		code(t, call(t, "POST", "/api/v1/assets", at, `{"asset_code":"M5-A","name":"x","category":"x","location":"x"}`, 409), "ASSET_CODE_CONFLICT")
		for _, tc := range []struct {
			body   string
			status int
		}{{`{"asset_code":"changed"}`, 422}, {`{}`, 422}, {`{"status":null}`, 422}, {`{"status":"BROKEN"}`, 422}, {`{"name":false}`, 400}, {`{"name":"x","name":"y"}`, 400}, {`{"name":"x"} {}`, 400}, {`{"name":" "}`, 422}} {
			call(t, "PATCH", "/api/v1/assets/"+a.ID, at, tc.body, tc.status)
		}
		call(t, "GET", "/api/v1/assets/bad", at, "", 422)
		for _, query := range []string{"?search=", "?status=active", "?page=1&page=2", "?assigned_to=" + tech.ID.String(), "?limit=101"} {
			call(t, "GET", "/api/v1/assets"+query, tt, "", 422)
		}
	})
	t.Run("literal filters pagination and no-op", func(t *testing.T) {
		for _, s := range []string{"%", "_", "\\", "PUMP 100"} {
			p := list(t, mt, "?search="+url.QueryEscape(s))
			if p.Meta.Total != 1 || len(p.Data) != 1 || p.Data[0].ID != a.ID {
				t.Fatal("literal search mismatch", s, p)
			}
		}
		if p := list(t, at, "?category=hvac"); p.Meta.Total != 0 {
			t.Fatal("category must be case sensitive")
		}
		if p := list(t, at, "?category=HVAC&status=INACTIVE"); p.Meta.Total != 0 {
			t.Fatal("filters not intersected")
		}
		if p := list(t, at, "?page=100&limit=1"); p.Meta.Total != 2 || len(p.Data) != 0 {
			t.Fatal(p)
		}
		var before int
		pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE entity_id=$1", a.ID).Scan(&before)
		got := decode(t, call(t, "PATCH", "/api/v1/assets/"+a.ID, mt, `{"category":" HVAC "}`, 200))
		var after int
		pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE entity_id=$1", a.ID).Scan(&after)
		if before != after || !got.UpdatedAt.Equal(a.UpdatedAt) {
			t.Fatal("no-op changed state")
		}
		call(t, "PATCH", "/api/v1/assets/"+b.ID, mt, `{"status":"MAINTENANCE","location":"Room B"}`, 200)
	})
	t.Run("scoped reads deduplicate terminal assignments", func(t *testing.T) {
		if p := list(t, tt, ""); p.Meta.Total != 0 {
			t.Fatal("unassigned visibility")
		}
		for i := 1; i <= 2; i++ {
			exec("INSERT INTO work_orders(work_order_number,asset_id,title,status,created_by,assigned_to) VALUES($1,$2,'Terminal fixture','CANCELLED',$3,$4)", fmt.Sprintf("WO-2026-00500%d", i), a.ID, admin.ID, tech.ID)
		}
		p := list(t, tt, "?limit=1")
		if p.Meta.Total != 1 || len(p.Data) != 1 || p.Data[0].ID != a.ID {
			t.Fatal("scope duplicated or leaked", p)
		}
		if p = list(t, tt, "?search=Other"); p.Meta.Total != 0 {
			t.Fatal("search widened scope")
		}
		call(t, "GET", "/api/v1/assets/"+a.ID, tt, "", 200)
		call(t, "GET", "/api/v1/assets/"+b.ID, tt, "", 403)
		code(t, call(t, "GET", "/api/v1/assets/00000000-0000-0000-0000-000000000001", tt, "", 404), "ASSET_NOT_FOUND")
		code(t, call(t, "DELETE", "/api/v1/assets/"+a.ID, at, "", 409), "ASSET_IN_USE")
		call(t, "PATCH", "/api/v1/assets/"+a.ID, mt, `{"status":"RETIRED"}`, 200)
		call(t, "PATCH", "/api/v1/assets/"+a.ID, mt, `{"status":"ACTIVE"}`, 200)
		exec("UPDATE work_orders SET status='ASSIGNED' WHERE asset_id=$1", a.ID)
		code(t, call(t, "PATCH", "/api/v1/assets/"+a.ID, mt, `{"status":"INACTIVE"}`, 409), "ASSET_HAS_OPEN_WORK_ORDERS")
		code(t, call(t, "PATCH", "/api/v1/assets/"+a.ID, mt, `{"status":"RETIRED"}`, 409), "ASSET_HAS_OPEN_WORK_ORDERS")
	})
	t.Run("audit rollback covers every mutation", func(t *testing.T) {
		exec(`CREATE FUNCTION reject_asset_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced audit failure'; END $$; CREATE TRIGGER reject_asset_test BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_asset_test()`)
		defer exec("DROP TRIGGER reject_asset_test ON audit_logs; DROP FUNCTION reject_asset_test()")
		call(t, "POST", "/api/v1/assets", at, `{"asset_code":"ROLLBACK","name":"x","category":"x","location":"x"}`, 500)
		call(t, "PATCH", "/api/v1/assets/"+b.ID, mt, `{"name":"Must rollback"}`, 500)
		call(t, "DELETE", "/api/v1/assets/"+b.ID, at, "", 500)
		got := decode(t, call(t, "GET", "/api/v1/assets/"+b.ID, at, "", 200))
		if got.Name != "Other pump" {
			t.Fatal("update escaped rollback")
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM assets WHERE asset_code='ROLLBACK'").Scan(&n); err != nil || n != 0 {
			t.Fatal("create escaped rollback", err)
		}
	})
	t.Run("delete keeps complete audit", func(t *testing.T) {
		w := call(t, "DELETE", "/api/v1/assets/"+b.ID, at, "", 204)
		if w.Body.Len() != 0 {
			t.Fatal("204 body")
		}
		call(t, "GET", "/api/v1/assets/"+b.ID, at, "", 404)
		var before string
		var after []byte
		var request string
		if err := pool.QueryRow(ctx, "SELECT old_value::text,new_value,request_id FROM audit_logs WHERE entity_id=$1 AND action='ASSET_DELETED'", b.ID).Scan(&before, &after, &request); err != nil {
			t.Fatal(err)
		}
		if after != nil || request != "m5-test" || !strings.Contains(before, "Other pump") {
			t.Fatal("delete audit contract")
		}
	})
	t.Run("retirement waits for concurrent order creation", func(t *testing.T) {
		c := create(t, "M5-RACE", "Concurrent asset")
		cid, _ := assets.ID(c.ID)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(ctx, "SELECT id FROM assets WHERE id=$1 FOR SHARE", cid); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := svc.Patch(ctx, admin.ID, cid, map[string]string{"status": "RETIRED"}, "asset-race")
			done <- err
		}()
		waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			var blocked bool
			err = pool.QueryRow(waitCtx, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE NOT granted AND locktype='transactionid')").Scan(&blocked)
			if err != nil {
				t.Fatal(err)
			}
			if blocked {
				break
			}
			select {
			case <-waitCtx.Done():
				t.Fatal("retirement never waited for asset lock")
			case <-tick.C:
			}
		}
		if _, err = tx.Exec(ctx, "INSERT INTO work_orders(work_order_number,asset_id,title,created_by) VALUES('WO-2026-005099',$1,'Concurrent order',$2)", cid, admin.ID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-done; !errors.Is(err, assets.OpenOrders) {
			t.Fatal("retirement missed committed order", err)
		}
	})
	// Same active-account guard applies to direct service calls.
	exec("UPDATE users SET is_active=false WHERE id=$1", manager.ID)
	if _, err := svc.Patch(ctx, manager.ID, aid, map[string]string{"name": "Denied"}, "inactive"); err == nil {
		t.Fatal("inactive actor wrote asset")
	}
}
