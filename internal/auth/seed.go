package auth

import (
	"context"
	"errors"
	"github.com/Gyebran/GoWork/internal/audit"
	"github.com/Gyebran/GoWork/internal/platform/database"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedDemo is explicit and development-only. Existing users are never overwritten.
func SeedDemo(ctx context.Context, pool *pgxpool.Pool, environment string) error {
	if environment != "development" && environment != "test" {
		return errors.New("demo seed requires development or test")
	}
	roles := []string{"ADMIN", "MANAGER", "TECHNICIAN"}
	emails := []string{"admin@gowork.dev", "manager@gowork.dev", "technician@gowork.dev"}
	hashes := make([]string, 3)
	for i := range roles {
		h, err := HashPassword("GoWork-demo-only-2026!")
		if err != nil {
			return err
		}
		hashes[i] = h
	}
	return database.InTx(ctx, pool, pgx.TxOptions{}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := q.LockUserAdministration(ctx); err != nil {
			return err
		}
		var adminID pgtype.UUID
		for i, role := range roles {
			existing, err := q.GetLoginUser(ctx, emails[i])
			if err == nil {
				if existing.Role != role || !existing.IsActive {
					return errors.New("demo account conflicts with existing user")
				}
				if i == 0 {
					adminID = existing.ID
				}
				continue
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if i == 0 {
				exists, err := q.HasAdministrator(ctx)
				if err != nil {
					return err
				}
				if exists {
					return ErrAlreadyBootstrapped
				}
			}
			u, err := q.InsertUser(ctx, dbsql.InsertUserParams{Name: "Demo " + role, Email: emails[i], PasswordHash: hashes[i], Role: role})
			if err != nil {
				return err
			}
			action := "USER_CREATED"
			actor := adminID
			if i == 0 {
				action = "USER_BOOTSTRAPPED"
				actor = pgtype.UUID{}
				adminID = u.ID
			}
			if err := audit.UserCreated(ctx, q, actor, u, role, action, "demo-"+u.ID.String()); err != nil {
				return err
			}
		}
		return nil
	})
}
