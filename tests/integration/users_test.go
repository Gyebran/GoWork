//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gyebran/GoWork/internal/auth"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/Gyebran/GoWork/internal/users"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func usersIntegration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
	svcAuth, err := auth.NewService(q, tokens)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ah := auth.NewHandler(svcAuth, logger)
	svc := users.NewService(pool)
	h := users.NewHandler(svc, ah, logger)
	router := httpx.NewRouterWithRoutes(logger, nil, func(r chi.Router) { ah.Register(r); h.Register(r) })
	adminToken, _ := tokens.Issue(admin.ID.String())
	managerToken, _ := tokens.Issue(manager.ID.String())
	techToken, _ := tokens.Issue(tech.ID.String())
	call := func(method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "m4-test")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "$2a$") {
			t.Fatal("hash in response")
		}
		return w
	}
	code := func(w *httptest.ResponseRecorder, want string) {
		t.Helper()
		if !strings.Contains(w.Body.String(), `"code":"`+want+`"`) {
			t.Fatal(w.Body.String())
		}
	}
	countAudits := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	payload := `{"name":"New Technician","email":"new@example.com","password":"integration-password-2026!","role":"TECHNICIAN"}`
	t.Run("permission matrix and live revocation", func(t *testing.T) {
		call("GET", "/api/v1/users", "", "", 401)
		call("GET", "/api/v1/users", managerToken, "", 200)
		call("GET", "/api/v1/users/"+admin.ID.String(), managerToken, "", 200)
		call("GET", "/api/v1/users", techToken, "", 403)
		call("GET", "/api/v1/users/not-a-uuid", techToken, "", 403)
		call("GET", "/api/v1/auth/me", techToken, "", 200)
		before := countAudits()
		call("POST", "/api/v1/users", managerToken, payload, 403)
		call("PATCH", "/api/v1/users/"+tech.ID.String(), managerToken, `{"name":"Denied"}`, 403)
		if countAudits() != before {
			t.Fatal("denied mutation audited")
		}
		exec("DELETE FROM role_permissions WHERE role_id=1 AND permission_id=2")
		call("GET", "/api/v1/users", adminToken, "", 403)
		exec("INSERT INTO role_permissions(role_id,permission_id) VALUES(1,2)")
	})
	var newUser auth.User
	t.Run("create conflict and validation", func(t *testing.T) {
		w := call("POST", "/api/v1/users", adminToken, payload, 201)
		var res struct {
			Data auth.User `json:"data"`
		}
		if json.Unmarshal(w.Body.Bytes(), &res) != nil {
			t.Fatal("response JSON")
		}
		newUser = res.Data
		if w.Header().Get("Location") != "/api/v1/users/"+newUser.ID || !newUser.IsActive {
			t.Fatal("create contract")
		}
		code(call("POST", "/api/v1/users", adminToken, strings.ReplaceAll(payload, "new@example.com", " NEW@example.com "), 409), "EMAIL_CONFLICT")
		for _, tc := range []struct {
			body   string
			status int
		}{{`{"name":"x","name":"y"}`, 400}, {`{"name":false}`, 400}, {`{"role":null}`, 422}, {`{"is_active":false}`, 422}, {`{"role":"UNKNOWN"}`, 422}, {`{} {}`, 400}} {
			call("POST", "/api/v1/users", adminToken, tc.body, tc.status)
		}
	})
	t.Run("pagination filters and unknown identifiers", func(t *testing.T) {
		w := call("GET", "/api/v1/users?role=TECHNICIAN&is_active=true&page=2&limit=1", managerToken, "", 200)
		var page users.Page
		if json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Data) != 1 || page.Meta.Total != 2 || page.Meta.TotalPages != 2 {
			t.Fatal(w.Body.String())
		}
		w = call("GET", "/api/v1/users?role=TECHNICIAN&page=100&limit=1", adminToken, "", 200)
		if !strings.Contains(w.Body.String(), `"data":[]`) || !strings.Contains(w.Body.String(), `"total":2`) {
			t.Fatal(w.Body.String())
		}
		for _, query := range []string{"role=admin", "is_active=1", "page=0", "page=1&page=2", "x=1", "limit=101"} {
			call("GET", "/api/v1/users?"+query, adminToken, "", 422)
		}
		call("GET", "/api/v1/users/bad", adminToken, "", 422)
		code(call("GET", "/api/v1/users/00000000-0000-0000-0000-000000000001", adminToken, "", 404), "USER_NOT_FOUND")
	})
	t.Run("patch no-op and sanitized audit", func(t *testing.T) {
		path := "/api/v1/users/" + newUser.ID
		before := countAudits()
		call("PATCH", path, adminToken, `{"name":"New Technician"}`, 200)
		if countAudits() != before {
			t.Fatal("no-op audited")
		}
		for _, tc := range []struct {
			body   string
			status int
		}{{`{}`, 422}, {`{"role":"ADMIN"}`, 422}, {`{"is_active":"false"}`, 400}, {`{"is_active":null}`, 422}, {`{"name":""}`, 422}, {`{"password":"short"}`, 422}} {
			call("PATCH", path, adminToken, tc.body, tc.status)
		}
		code(call("PATCH", path, adminToken, `{"email":"admin@gowork.dev"}`, 409), "EMAIL_CONFLICT")
		call("PATCH", path, adminToken, `{"password":"changed-password-2026!"}`, 200)
		var raw string
		if err := pool.QueryRow(ctx, "SELECT new_value::text FROM audit_logs WHERE entity_id=$1 AND action='USER_UPDATED'", newUser.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(raw, `"password_changed": true`) || strings.Contains(raw, "changed-password") || strings.Contains(raw, "$2") {
			t.Fatal("unsafe audit")
		}
		call("POST", "/api/v1/auth/login", "", `{"email":"new@example.com","password":"changed-password-2026!"}`, 200)
		token, _ := tokens.Issue(newUser.ID)
		call("PATCH", path, adminToken, `{"is_active":false}`, 200)
		call("GET", "/api/v1/auth/me", token, "", 401)
		call("PATCH", path, adminToken, `{"is_active":true}`, 200)
		code(call("PATCH", "/api/v1/users/"+admin.ID.String(), adminToken, `{"is_active":false}`, 409), "SELF_DEACTIVATION")
	})
	t.Run("audit failure rolls back create and update", func(t *testing.T) {
		exec(`CREATE FUNCTION reject_user_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced audit failure'; END $$; CREATE TRIGGER reject_user_audit_test BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_user_audit_test()`)
		defer exec("DROP TRIGGER reject_user_audit_test ON audit_logs; DROP FUNCTION reject_user_audit_test()")
		call("POST", "/api/v1/users", adminToken, strings.ReplaceAll(payload, "new@example.com", "rollback@example.com"), 500)
		call("PATCH", "/api/v1/users/"+newUser.ID, adminToken, `{"name":"Must Rollback"}`, 500)
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE email='rollback@example.com' OR name='Must Rollback'").Scan(&n); err != nil || n != 0 {
			t.Fatal("partial mutation", err)
		}
	})
	t.Run("concurrent deactivations cannot disable both administrators", func(t *testing.T) {
		second, err := svc.Create(ctx, admin.ID, users.CreateInput{Name: "Second Admin", Email: "second@example.com", Password: "integration-password-2026!", Role: "ADMIN"}, "concurrent-admin")
		if err != nil {
			t.Fatal(err)
		}
		secondID, _ := users.ID(second.ID)
		inactive := false
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := svc.Patch(ctx, admin.ID, secondID, users.PatchInput{IsActive: &inactive}, "concurrent-admin")
			results <- err
		}()
		go func() {
			<-start
			_, err := svc.Patch(ctx, secondID, admin.ID, users.PatchInput{IsActive: &inactive}, "concurrent-admin")
			results <- err
		}()
		close(start)
		a, b := <-results, <-results
		if (a == nil) == (b == nil) {
			t.Fatal("expected exactly one success", a, b)
		}
		n, err := q.CountActiveAdmins(ctx)
		if err != nil || n != 1 {
			t.Fatal("lost last admin", err)
		}
		exec("UPDATE users SET is_active=true WHERE id=$1", admin.ID)
	})
	t.Run("assignment lock serializes with deactivation", func(t *testing.T) {
		var asset pgtype.UUID
		if err := pool.QueryRow(ctx, "INSERT INTO assets(asset_code,name,category,location) VALUES('M4-TEST','Test asset','Test','Test') RETURNING id").Scan(&asset); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(ctx, "SELECT id FROM users WHERE id=$1 FOR SHARE", tech.ID); err != nil {
			t.Fatal(err)
		}
		inactive := false
		done := make(chan error, 1)
		go func() {
			_, err := svc.Patch(ctx, admin.ID, tech.ID, users.PatchInput{IsActive: &inactive}, "assignment-race")
			done <- err
		}()
		// Observe the lock wait itself before committing the assignment. No timing-based assumption.
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
				t.Fatal("deactivation never waited for assignment lock")
			case <-tick.C:
			}
		}
		if _, err = tx.Exec(ctx, "INSERT INTO work_orders(work_order_number,asset_id,title,status,created_by,assigned_to) VALUES('WO-2026-000999',$1,'Locked assignment','ASSIGNED',$2,$3)", asset, admin.ID, tech.ID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-done; !errors.Is(err, users.ActiveOrders) {
			t.Fatal("active assignment was missed", err)
		}
		exec("UPDATE work_orders SET status='CANCELLED' WHERE work_order_number='WO-2026-000999'")
		call("PATCH", "/api/v1/users/"+tech.ID.String(), adminToken, `{"is_active":false}`, 200)
		// Future assignment must lock and re-read the disabled target before inserting.
		tx, err = pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		var active bool
		if err = tx.QueryRow(ctx, "SELECT is_active FROM users WHERE id=$1 FOR SHARE", tech.ID).Scan(&active); err != nil || active {
			t.Fatal("inactive target escaped lock check", err)
		}
	})
}
