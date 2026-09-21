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
	"strings"
	"testing"
	"time"

	"github.com/Gyebran/GoWork/internal/auth"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/Gyebran/GoWork/internal/workorders"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func workOrdersIntegration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
	other, err := q.InsertUser(ctx, dbsql.InsertUserParams{Name: "Other technician", Email: "other-tech@example.com", PasswordHash: tech.PasswordHash, Role: "TECHNICIAN"})
	if err != nil {
		t.Fatal(err)
	}
	var asset pgtype.UUID
	if err = pool.QueryRow(ctx, "INSERT INTO assets(asset_code,name,category,location) VALUES('M6','M6 asset','Test','Test') RETURNING id").Scan(&asset); err != nil {
		t.Fatal(err)
	}
	tokens, _ := auth.NewTokens("integration-signing-key-with-at-least-32-bytes", "gowork", "gowork-api")
	as, err := auth.NewService(q, tokens)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ah := auth.NewHandler(as, logger)
	svc := workorders.NewService(pool)
	h := workorders.NewHandler(svc, ah, logger)
	router := httpx.NewRouterWithRoutes(logger, nil, func(r chi.Router) { ah.Register(r); h.Register(r) })
	at, _ := tokens.Issue(admin.ID.String())
	mt, _ := tokens.Issue(manager.ID.String())
	tt, _ := tokens.Issue(tech.ID.String())
	ot, _ := tokens.Issue(other.ID.String())
	call := func(t *testing.T, method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "m6-test")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	decode := func(t *testing.T, w *httptest.ResponseRecorder) workorders.Order {
		t.Helper()
		var d struct {
			Data workorders.Order `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d.Data
	}
	code := func(t *testing.T, w *httptest.ResponseRecorder, c string) {
		t.Helper()
		if !strings.Contains(w.Body.String(), `"code":"`+c+`"`) {
			t.Fatal(w.Body.String())
		}
	}
	payload := fmt.Sprintf(`{"asset_id":"%s","title":"Test order"}`, asset.String())
	create := func(t *testing.T) workorders.Order {
		t.Helper()
		w := call(t, "POST", "/api/v1/work-orders", mt, payload, 201)
		o := decode(t, w)
		if w.Header().Get("Location") != "/api/v1/work-orders/"+o.ID || o.Status != "OPEN" || o.Priority != "MEDIUM" || o.CreatedBy != manager.ID.String() || o.AssignedTo != nil || o.CompletedAt != nil {
			t.Fatal("creation contract", w.Body.String())
		}
		return o
	}
	path := func(o workorders.Order) string { return "/api/v1/work-orders/" + o.ID }
	assign := func(t *testing.T, o workorders.Order, target pgtype.UUID, want int) *httptest.ResponseRecorder {
		return call(t, "PATCH", path(o)+"/assign", mt, fmt.Sprintf(`{"assigned_to":"%s"}`, target.String()), want)
	}
	status := func(t *testing.T, o workorders.Order, token, to string, want int) *httptest.ResponseRecorder {
		return call(t, "PATCH", path(o)+"/status", token, fmt.Sprintf(`{"status":"%s"}`, to), want)
	}
	audits := func(t *testing.T, id string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE entity_id=$1", id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	o := create(t)
	t.Run("creation authorization and strict server fields", func(t *testing.T) {
		call(t, "POST", "/api/v1/work-orders", "", payload, 401)
		call(t, "POST", "/api/v1/work-orders", tt, payload, 403)
		for _, field := range []string{`"status":"OPEN"`, `"created_by":"` + admin.ID.String() + `"`, `"assigned_to":"` + tech.ID.String() + `"`, `"work_order_number":"WO-2026-000001"`} {
			call(t, "POST", "/api/v1/work-orders", at, strings.TrimSuffix(payload, "}")+","+field+"}", 422)
		}
		call(t, "POST", "/api/v1/work-orders", at, `{"title":true}`, 400)
		call(t, "POST", "/api/v1/work-orders", at, `{"title":"a","title":"b"}`, 400)
		code(t, call(t, "POST", "/api/v1/work-orders", at, strings.ReplaceAll(payload, asset.String(), "00000000-0000-0000-0000-000000000001"), 404), "ASSET_NOT_FOUND")
		for _, state := range []string{"INACTIVE", "RETIRED"} {
			exec("UPDATE assets SET status=$1 WHERE id=$2", state, asset)
			code(t, call(t, "POST", "/api/v1/work-orders", at, payload, 409), "ASSET_NOT_AVAILABLE")
		}
		exec("UPDATE assets SET status='MAINTENANCE' WHERE id=$1", asset)
		create(t)
		exec("UPDATE assets SET status='ACTIVE' WHERE id=$1", asset)
	})
	t.Run("assignment ownership and lifecycle", func(t *testing.T) {
		code(t, status(t, o, mt, "COMPLETED", 409), "INVALID_STATUS_TRANSITION")
		for _, id := range []pgtype.UUID{admin.ID, manager.ID, {Bytes: [16]byte{1}, Valid: true}} {
			code(t, assign(t, o, id, 409), "INVALID_ASSIGNEE")
		}
		exec("UPDATE users SET is_active=false WHERE id=$1", other.ID)
		code(t, assign(t, o, other.ID, 409), "INVALID_ASSIGNEE")
		exec("UPDATE users SET is_active=true WHERE id=$1", other.ID)
		call(t, "PATCH", path(o)+"/assign", mt, `{"assigned_to":null}`, 422)
		before := audits(t, o.ID)
		assign(t, o, tech.ID, 200)
		if audits(t, o.ID) != before+2 {
			t.Fatal("OPEN assignment must have two audits")
		}
		code(t, assign(t, o, tech.ID, 409), "ALREADY_ASSIGNED")
		call(t, "GET", path(o), tt, "", 200)
		call(t, "GET", path(o), ot, "", 403)
		status(t, o, ot, "IN_PROGRESS", 403)
		status(t, o, tt, "CANCELLED", 403)
		assign(t, o, other.ID, 200)
		call(t, "GET", path(o), tt, "", 403)
		assign(t, o, tech.ID, 200)
		call(t, "PATCH", path(o), tt, `{"title":"Denied"}`, 403)
		before = audits(t, o.ID)
		prior := decode(t, call(t, "GET", path(o), mt, "", 200))
		got := decode(t, call(t, "PATCH", path(o), mt, `{"title":"Test order"}`, 200))
		if audits(t, o.ID) != before || !got.UpdatedAt.Equal(prior.UpdatedAt) {
			t.Fatal("metadata no-op audit")
		}
		call(t, "PATCH", path(o), mt, `{"description":"Details","priority":"HIGH"}`, 200)
		call(t, "PATCH", path(o), mt, `{"description":""}`, 200)
		call(t, "PATCH", path(o), mt, `{"status":"COMPLETED"}`, 422)
		call(t, "PATCH", path(o), mt, `{}`, 422)
		status(t, o, tt, "IN_PROGRESS", 200)
		code(t, assign(t, o, other.ID, 409), "INVALID_ASSIGNMENT_STATE")
		code(t, status(t, o, tt, "IN_PROGRESS", 409), "INVALID_STATUS_TRANSITION")
		done := decode(t, status(t, o, tt, "COMPLETED", 200))
		if done.CompletedAt == nil || done.AssignedTo == nil {
			t.Fatal("completion fields")
		}
		code(t, status(t, o, tt, "COMPLETED", 409), "WORK_ORDER_TERMINAL")
		code(t, call(t, "PATCH", path(o), mt, `{"title":"No"}`, 409), "WORK_ORDER_TERMINAL")
		code(t, assign(t, o, other.ID, 409), "WORK_ORDER_TERMINAL")
		status(t, o, ot, "COMPLETED", 403)
	})
	t.Run("cancel and scoped pagination", func(t *testing.T) {
		for _, from := range []string{"OPEN", "ASSIGNED", "IN_PROGRESS"} {
			x := create(t)
			if from != "OPEN" {
				assign(t, x, tech.ID, 200)
			}
			if from == "IN_PROGRESS" {
				status(t, x, tt, "IN_PROGRESS", 200)
			}
			c := decode(t, status(t, x, mt, "CANCELLED", 200))
			if c.CompletedAt != nil || (from != "OPEN" && c.AssignedTo == nil) {
				t.Fatal("cancel history")
			}
		}
		w := call(t, "GET", "/api/v1/work-orders?status=COMPLETED&priority=HIGH&asset_id="+asset.String()+"&assigned_to="+tech.ID.String()+"&limit=1", tt, "", 200)
		var page workorders.Page
		if json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Meta.Total != 1 || len(page.Data) != 1 {
			t.Fatal(w.Body.String())
		}
		w = call(t, "GET", "/api/v1/work-orders?status=COMPLETED&page=99&limit=1", tt, "", 200)
		if !strings.Contains(w.Body.String(), `"data":[]`) || !strings.Contains(w.Body.String(), `"total":1`) {
			t.Fatal(w.Body.String())
		}
		call(t, "GET", "/api/v1/work-orders?assigned_to="+other.ID.String(), tt, "", 403)
		call(t, "GET", "/api/v1/work-orders?assigned_to="+strings.ToUpper(tech.ID.String()), tt, "", 200)
		call(t, "GET", "/api/v1/work-orders?status=OPEN&status=ASSIGNED", mt, "", 422)
		code(t, call(t, "GET", "/api/v1/work-orders/00000000-0000-0000-0000-000000000001", tt, "", 404), "WORK_ORDER_NOT_FOUND")
	})
	t.Run("audit failure undoes creation metadata status and both assignment events", func(t *testing.T) {
		x := create(t)
		before := audits(t, x.ID)
		exec(`CREATE FUNCTION reject_order_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action <> 'WORK_ORDER_ASSIGNED' THEN RAISE EXCEPTION 'forced audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_order_test BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_order_test()`)
		defer exec("DROP TRIGGER reject_order_test ON audit_logs; DROP FUNCTION reject_order_test()")
		var countBefore, countAfter int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM work_orders").Scan(&countBefore); err != nil {
			t.Fatal(err)
		}
		call(t, "POST", "/api/v1/work-orders", mt, payload, 500)
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM work_orders").Scan(&countAfter); err != nil || countAfter != countBefore {
			t.Fatal("create escaped rollback", err)
		}
		assign(t, x, tech.ID, 500)
		call(t, "PATCH", path(x), mt, `{"title":"Rollback"}`, 500)
		status(t, x, mt, "CANCELLED", 500)
		got := decode(t, call(t, "GET", path(x), mt, "", 200))
		if got.Status != "OPEN" || got.AssignedTo != nil || got.Title != "Test order" || audits(t, x.ID) != before {
			t.Fatal("partial write/audit escaped rollback")
		}
	})
	t.Run("simultaneous completion and unique untruncated numbers", func(t *testing.T) {
		x := create(t)
		assign(t, x, tech.ID, 200)
		status(t, x, tt, "IN_PROGRESS", 200)
		id, _ := workorders.ID(x.ID)
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				_, err := svc.Status(ctx, tech.ID, id, "COMPLETED", "concurrent-completion")
				results <- err
			}()
		}
		close(start)
		a, b := <-results, <-results
		if (a == nil) == (b == nil) {
			t.Fatal(a, b)
		}
		if a != nil && !errors.Is(a, workorders.Terminal) {
			t.Fatal(a)
		}
		if b != nil && !errors.Is(b, workorders.Terminal) {
			t.Fatal(b)
		}
		var completedAudits int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE entity_id=$1 AND new_value->>'status'='COMPLETED'", x.ID).Scan(&completedAudits); err != nil || completedAudits != 1 {
			t.Fatal("duplicate completion audit", err)
		}
		exec("SELECT setval('work_order_number_seq',999999,true)")
		type result struct {
			order workorders.Order
			err   error
		}
		made := make(chan result, 6)
		start = make(chan struct{})
		for range 6 {
			go func() {
				<-start
				o, err := svc.Create(ctx, manager.ID, map[string]string{"asset_id": asset.String(), "title": "Concurrent"}, "concurrent-create")
				made <- result{o, err}
			}()
		}
		close(start)
		seen := map[string]bool{}
		for range 6 {
			r := <-made
			if r.err != nil {
				t.Fatal(r.err)
			}
			if seen[r.order.Number] || len(strings.Split(r.order.Number, "-")[2]) != 7 {
				t.Fatal("duplicate or truncated number", r.order.Number)
			}
			seen[r.order.Number] = true
		}
	})
	t.Run("assignment rechecks technician after waiting for deactivation", func(t *testing.T) {
		x := create(t)
		id, _ := workorders.ID(x.ID)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(ctx, "UPDATE users SET is_active=false WHERE id=$1", other.ID); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := svc.Assign(ctx, manager.ID, id, other.ID, "deactivation-race"); done <- err }()
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
				t.Fatal("assignment did not wait")
			case <-tick.C:
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-done; !errors.Is(err, workorders.InvalidAssignee) {
			t.Fatal("assigned inactive technician", err)
		}
	})

	t.Run("creation waits for asset retirement", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(ctx, "UPDATE assets SET status='RETIRED' WHERE id=$1", asset); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := svc.Create(ctx, manager.ID, map[string]string{"asset_id": asset.String(), "title": "Must reject retired"}, "retirement-race")
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
				t.Fatal("creation did not wait")
			case <-tick.C:
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-done; !errors.Is(err, workorders.AssetUnavailable) {
			t.Fatal("created on retired asset", err)
		}
	})
}
