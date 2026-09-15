//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Gyebran/GoWork/internal/platform/database"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/Gyebran/GoWork/internal/platform/migrations"
	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseFoundation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if err := migrations.ValidateTestURL(dsn, os.Getenv("APP_ENV")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn, 5, false)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	// Refuse to destroy any existing application tables: only a fresh test DB is eligible.
	var existing int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('users','assets','work_orders','roles','audit_logs','permissions','role_permissions')").Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != 0 {
		t.Fatal("integration requires fresh test DB; will not destroy existing application tables")
	}
	m, err := migrations.Open(dsn, "../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		a, b := m.Close()
		if a != nil || b != nil {
			t.Errorf("migration cleanup: %v %v", a, b)
		}
	}()
	if database.Ready(ctx, pool) == nil {
		t.Fatal("unmigrated database reported ready")
	}
	for _, direction := range []string{"up", "down", "up"} {
		if direction == "up" {
			err = m.Up()
		} else {
			err = m.Down()
		}
		if err != nil {
			t.Fatalf("migration %s failed: %v", direction, err)
		}
	}
	defer func() {
		if err := m.Down(); err != nil {
			t.Errorf("cleanup down: %v", err)
		}
	}()
	if err = m.Up(); !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("second up: %v", err)
	}
	if err = database.Ready(ctx, pool); err != nil {
		t.Fatal(err)
	}
	q := dbsql.New(pool)
	roles, err := q.ListRoles(ctx)
	if err != nil || len(roles) != 3 {
		t.Fatalf("roles: %v", err)
	}
	permissions, err := q.ListPermissions(ctx)
	if err != nil || len(permissions) != 14 {
		t.Fatalf("permissions: %v", err)
	}
	mappings, err := q.ListRolePermissions(ctx)
	if err != nil || len(mappings) != 28 {
		t.Fatalf("mappings: %v %d", err, len(mappings))
	}
	expected := map[int16][]int16{1: {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, 2: {2, 4, 5, 6, 8, 9, 10, 11, 12, 13, 14}, 3: {5, 9, 12}}
	for _, m := range mappings {
		found := false
		for _, id := range expected[m.RoleID] {
			if id == m.PermissionID {
				found = true
			}
		}
		if !found {
			t.Fatalf("unexpected grant %+v", m)
		}
	}
	t.Run("readiness HTTP and dirty schema", func(t *testing.T) {
		ready := httpx.NewReadiness(func(c context.Context) error { return database.Ready(c, pool) })
		router := httpx.NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), ready)
		for _, dirty := range []bool{false, true} {
			if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET dirty=$1", dirty); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/ready", nil))
			want := 200
			if dirty {
				want = 503
			}
			if w.Code != want {
				t.Fatal(w.Code)
			}
		}
		if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET dirty=false"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("constraints", func(t *testing.T) {
		statements := []struct{ sql, code string }{
			{"INSERT INTO assets(asset_code,name,category,location,status) VALUES('BAD','x','x','x','UNKNOWN')", "23514"},
			{"INSERT INTO users(name,email,password_hash,role_id) VALUES('test','test@example.com',repeat('x',60),99)", "23503"},
			{"INSERT INTO audit_logs(action,entity_type,entity_id,new_value,request_id) VALUES('USER_CREATED','user',gen_random_uuid(),'{}','test')", "23514"},
			{"INSERT INTO audit_logs(action,entity_type,entity_id,new_value,request_id) VALUES('USER_BOOTSTRAPPED','user',gen_random_uuid(),'[]','test')", "23514"},
		}
		for _, s := range statements {
			_, err := pool.Exec(ctx, s.sql)
			var e *pgconn.PgError
			if !errors.As(err, &e) || e.Code != s.code {
				t.Fatalf("expected %s: %v", s.code, err)
			}
		}
	})
	t.Run("commit and audit rollback", func(t *testing.T) {
		err := database.InTx(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx, q *dbsql.Queries) error {
			if _, err := q.ListRoles(ctx); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO assets(asset_code,name,category,location) VALUES('COMMIT','test','test','test')")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		err = database.InTx(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx, _ *dbsql.Queries) error {
			if _, err := tx.Exec(ctx, "INSERT INTO assets(asset_code,name,category,location) VALUES('ROLLBACK','test','test','test')"); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO audit_logs(action,entity_type,entity_id,new_value,request_id) VALUES('INVALID','asset',gen_random_uuid(),'{}','test')")
			return err
		})
		if err == nil {
			t.Fatal("audit failure ignored")
		}
		var count int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM assets WHERE asset_code='ROLLBACK'").Scan(&count); err != nil || count != 0 {
			t.Fatal("rollback failed")
		}
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM assets WHERE asset_code='COMMIT'").Scan(&count); err != nil || count != 1 {
			t.Fatal("commit failed")
		}
	})
	t.Run("runtime audit permissions", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		for _, sql := range []string{
			"CREATE ROLE gowork_test_runtime NOLOGIN",
			"GRANT USAGE ON SCHEMA public TO gowork_test_runtime",
			"GRANT SELECT, INSERT ON audit_logs TO gowork_test_runtime",
			"SET LOCAL ROLE gowork_test_runtime",
			"INSERT INTO audit_logs(action,entity_type,entity_id,new_value,request_id) VALUES('USER_BOOTSTRAPPED','user',gen_random_uuid(),'{}','test')",
		} {
			if _, err := tx.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
		}
		for _, sql := range []string{"UPDATE audit_logs SET request_id='bad'", "DELETE FROM audit_logs"} {
			if _, err := tx.Exec(ctx, "SAVEPOINT permission_test"); err != nil {
				t.Fatal(err)
			}
			_, err := tx.Exec(ctx, sql)
			var e *pgconn.PgError
			if !errors.As(err, &e) || e.Code != "42501" {
				t.Fatalf("audit mutation allowed: %v", err)
			}
			if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT permission_test"); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("cancellation rollback", func(t *testing.T) {
		c, stop := context.WithCancel(ctx)
		err := database.InTx(c, pool, pgx.TxOptions{}, func(tx pgx.Tx, _ *dbsql.Queries) error {
			_, err := tx.Exec(c, "INSERT INTO assets(asset_code,name,category,location) VALUES('CANCEL','test','test','test')")
			stop()
			if err != nil {
				return err
			}
			return c.Err()
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		var count int
		err = pool.QueryRow(ctx, "SELECT count(*) FROM assets WHERE asset_code='CANCEL'").Scan(&count)
		if err != nil || count != 0 {
			t.Fatal("cancelled write persisted")
		}
	})
}

func TestTestDatabaseGuard(t *testing.T) {
	for _, tt := range []struct {
		dsn, env string
		ok       bool
	}{{"postgres://localhost/gowork_test", "test", true}, {"postgres://localhost/gowork", "test", false}, {"postgres://localhost/gowork_test", "production", false}, {"", "test", false}, {"postgres://localhost/gowork_test?database=production", "test", false}} {
		if err := migrations.ValidateTestURL(tt.dsn, tt.env); (err == nil) != tt.ok {
			t.Fatalf("unexpected guard result %v", err)
		}
	}
}
