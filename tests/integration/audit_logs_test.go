//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gyebran/GoWork/internal/assets"
	"github.com/Gyebran/GoWork/internal/auditlog"
	"github.com/Gyebran/GoWork/internal/auth"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/Gyebran/GoWork/internal/users"
	"github.com/Gyebran/GoWork/internal/workorders"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func auditLogsIntegration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
	manager, err := q.GetLoginUser(ctx, "manager@gowork.dev")
	if err != nil {
		t.Fatal(err)
	}
	tech, err := q.GetLoginUser(ctx, "technician@gowork.dev")
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
	svc := auditlog.NewService(pool)
	h := auditlog.NewHandler(svc, ah, logger)
	router := httpx.NewRouterWithRoutes(logger, nil, func(r chi.Router) { ah.Register(r); h.Register(r) })
	at, _ := tokens.Issue(admin.ID.String())
	mt, _ := tokens.Issue(manager.ID.String())
	tt, _ := tokens.Issue(tech.ID.String())
	call := func(t *testing.T, token, query string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/audit-logs"+query, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s got %d want %d %s", query, w.Code, want, w.Body.String())
		}
		return w
	}
	page := func(t *testing.T, token, query string) auditlog.Page {
		t.Helper()
		w := call(t, token, query, 200)
		var p auditlog.Page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Run("permission gate and live revocation", func(t *testing.T) {
		call(t, "", "", 401)
		call(t, tt, "", 403)
		call(t, tt, "?invalid=x", 403)
		if p := page(t, mt, ""); p.Meta.Total != 3 {
			t.Fatal(p)
		}
		exec("DELETE FROM role_permissions WHERE role_id=1 AND permission_id=14")
		call(t, at, "", 403)
		exec("INSERT INTO role_permissions(role_id,permission_id) VALUES(1,14)")
		exec("UPDATE users SET is_active=false WHERE id=$1", manager.ID)
		call(t, mt, "", 401)
		exec("UPDATE users SET is_active=true WHERE id=$1", manager.ID)
	})
	t.Run("filters pagination stable ties and bootstrap nulls", func(t *testing.T) {
		p := page(t, at, "?action=USER_BOOTSTRAPPED")
		if len(p.Data) != 1 || p.Data[0].ActorID != nil || string(p.Data[0].OldValue) != "null" {
			t.Fatal("bootstrap serialization", p)
		}
		// The seed's three audit rows share one transaction timestamp; UUID breaks the tie.
		first := page(t, mt, "?limit=1")
		second := page(t, mt, "?page=2&limit=1")
		if first.Meta.Total != 3 || first.Meta.TotalPages != 3 || first.Data[0].ID <= second.Data[0].ID {
			t.Fatal("unstable tie ordering")
		}
		p = page(t, mt, "?page=100&limit=1")
		if len(p.Data) != 0 || p.Meta.Total != 3 {
			t.Fatal("empty-page total")
		}
		p = page(t, mt, "?actor_id="+admin.ID.String()+"&entity_type=user&action=USER_CREATED&entity_id="+tech.ID.String())
		if p.Meta.Total != 1 || p.Data[0].EntityID != tech.ID.String() {
			t.Fatal("filters not ANDed", p)
		}
		p = page(t, mt, "?entity_type=asset&action=USER_UPDATED")
		if p.Meta.Total != 0 {
			t.Fatal("valid incompatible filters must be empty")
		}
		for _, query := range []string{"?entity_id=" + tech.ID.String(), "?actor_id=null", "?entity_type=users", "?action=LOGIN", "?page=0", "?limit=101", "?page=1&page=2", "?sort=id"} {
			call(t, at, query, 422)
		}
	})
	t.Run("real mutation snapshots and retained history", func(t *testing.T) {
		password := "audit-test-password-2026!"
		_, err := users.NewService(pool).Patch(ctx, admin.ID, tech.ID, users.PatchInput{Password: &password}, "audit-password")
		if err != nil {
			t.Fatal(err)
		}
		p := page(t, mt, "?entity_type=user&entity_id="+tech.ID.String()+"&action=USER_UPDATED")
		if len(p.Data) != 1 || p.Data[0].RequestID != "audit-password" || !strings.Contains(string(p.Data[0].NewValue), `"password_changed":true`) || strings.Contains(string(p.Data[0].NewValue), password) {
			t.Fatal("password audit contract", p)
		}
		assetSvc := assets.NewService(pool)
		a, err := assetSvc.Create(ctx, admin.ID, map[string]string{"asset_code": "M7", "name": "Audit asset", "category": "Test", "location": "Test"}, "audit-asset")
		if err != nil {
			t.Fatal(err)
		}
		aid, _ := assets.ID(a.ID)
		if err = assetSvc.Delete(ctx, admin.ID, aid, "audit-delete"); err != nil {
			t.Fatal(err)
		}
		p = page(t, mt, "?entity_type=asset&entity_id="+a.ID+"&action=ASSET_DELETED")
		if p.Meta.Total != 1 || string(p.Data[0].NewValue) != "null" || !strings.Contains(string(p.Data[0].OldValue), `"asset_code":"M7"`) {
			t.Fatal("lost deleted snapshot", p)
		}
		a, err = assetSvc.Create(ctx, admin.ID, map[string]string{"asset_code": "M7-WO", "name": "Order asset", "category": "Test", "location": "Test"}, "audit-order")
		if err != nil {
			t.Fatal(err)
		}
		orders := workorders.NewService(pool)
		o, err := orders.Create(ctx, admin.ID, map[string]string{"asset_id": a.ID, "title": "Audit order"}, "audit-order")
		if err != nil {
			t.Fatal(err)
		}
		oid, _ := workorders.ID(o.ID)
		if _, err = orders.Assign(ctx, admin.ID, oid, tech.ID, "assignment-pair"); err != nil {
			t.Fatal(err)
		}
		p = page(t, mt, "?entity_type=work_order&entity_id="+o.ID)
		if p.Meta.Total != 3 {
			t.Fatal("assignment audits missing")
		}
		paired := 0
		for _, e := range p.Data {
			if e.RequestID == "assignment-pair" {
				paired++
				if !strings.Contains(string(e.OldValue), `"status":"OPEN"`) || !strings.Contains(string(e.NewValue), `"status":"ASSIGNED"`) {
					t.Fatal("assignment snapshots")
				}
			}
		}
		if paired != 2 {
			t.Fatal("request correlation")
		}
	})
	t.Run("legacy secret fields stripped and invalid public types fail closed", func(t *testing.T) {
		exec(`UPDATE audit_logs SET new_value=new_value || '{"password":"fixture-secret","password_hash":"fixture-hash","token":"fixture-token","extra":{"secret":"fixture-nested"}}'::jsonb WHERE entity_type='user'`)
		w := call(t, at, "?entity_type=user", 200)
		for _, secret := range []string{"fixture-secret", "fixture-hash", "fixture-token", "fixture-nested"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("secret leaked")
			}
		}
		exec(`UPDATE audit_logs SET new_value=jsonb_set(new_value,'{name}','{"password":"nested-public-secret"}'::jsonb) WHERE action='USER_BOOTSTRAPPED'`)
		w = call(t, at, "?action=USER_BOOTSTRAPPED", 500)
		if strings.Contains(w.Body.String(), "nested-public-secret") {
			t.Fatal("unsafe error response")
		}
	})
}
