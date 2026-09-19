//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Gyebran/GoWork/internal/auth"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func authIntegration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	cleanup := func() {
		if _, err := pool.Exec(ctx, "DELETE FROM audit_logs; DELETE FROM users"); err != nil {
			t.Error(err)
		}
	}
	defer cleanup()
	// A real audit insert failure must undo the just-created administrator.
	_, err := pool.Exec(ctx, `CREATE FUNCTION reject_bootstrap_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced audit failure'; END $$;
 CREATE TRIGGER reject_bootstrap_test BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_bootstrap_test();`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = auth.Bootstrap(ctx, pool, "Admin", "admin@example.com", "integration-password-2026!")
	if err == nil {
		t.Fatal("bootstrap succeeded despite failed audit")
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER reject_bootstrap_test ON audit_logs; DROP FUNCTION reject_bootstrap_test()"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 0 {
		t.Fatal("audit failure left a user behind")
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := auth.Bootstrap(ctx, pool, "Admin", " ADMIN@example.com ", "integration-password-2026!")
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, auth.ErrAlreadyBootstrapped) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("bootstrap concurrency guard failed")
	}
	var snapshot string
	if err = pool.QueryRow(ctx, "SELECT new_value::text FROM audit_logs WHERE action='USER_BOOTSTRAPPED'").Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot, "password") || strings.Contains(snapshot, "$2") {
		t.Fatal("secret in audit snapshot")
	}
	tokens, err := auth.NewTokens("integration-signing-key-with-at-least-32-bytes", "gowork", "gowork-api")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(dbsql.New(pool), tokens)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := auth.NewHandler(svc, logger)
	router := httpx.NewRouterWithRoutes(logger, nil, h.Register)
	login := func(email, password string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]string{"email": email, "password": password})
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(string(b)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	me := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	w := login("ADMIN@example.com", "integration-password-2026!")
	if w.Code != 200 {
		t.Fatalf("login %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Data auth.LoginResult `json:"data"`
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal("invalid login response")
	}
	if strings.Contains(w.Body.String(), "password") || result.Data.ExpiresIn != 900 {
		t.Fatal("unsafe login response")
	}
	if w = me(result.Data.AccessToken); w.Code != 200 {
		t.Fatalf("me %d", w.Code)
	}
	for _, email := range []string{"admin@example.com", "missing@example.com"} {
		w = login(email, "wrong-password")
		if w.Code != 401 || !strings.Contains(w.Body.String(), "INVALID_CREDENTIALS") {
			t.Fatal("credential error contract")
		}
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET is_active=false WHERE email='admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	if w = me(result.Data.AccessToken); w.Code != 401 {
		t.Fatal("inactive user token accepted")
	}
	w = login("admin@example.com", "integration-password-2026!")
	if w.Code != 401 || !strings.Contains(w.Body.String(), "INVALID_CREDENTIALS") {
		t.Fatal("inactive login accepted")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET is_active=true,role_id=2 WHERE email='admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	w = me(result.Data.AccessToken)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"MANAGER"`) {
		t.Fatal("role not read from database")
	}
	unknown, _ := tokens.Issue("de67a3be-9b67-4312-a156-3f284206f963")
	if me(unknown).Code != 401 {
		t.Fatal("unknown user token accepted")
	}
	cleanup()
	if err = auth.SeedDemo(ctx, pool, "production"); err == nil {
		t.Fatal("production seed accepted")
	}
	for range 2 {
		if err = auth.SeedDemo(ctx, pool, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 3 {
		t.Fatal("demo seed not idempotent")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs").Scan(&count); err != nil || count != 3 {
		t.Fatal("duplicate seed audits")
	}
	if login("technician@gowork.dev", "GoWork-demo-only-2026!").Code != 200 {
		t.Fatal("demo password hashing failed")
	}
}
