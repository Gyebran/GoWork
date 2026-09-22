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
	"strings"
	"unicode/utf8"
)

var ErrAlreadyBootstrapped = errors.New("administrator already exists")

func Bootstrap(ctx context.Context, pool *pgxpool.Pool, name, email, password string) (User, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || strings.ContainsRune(name, 0) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 {
		return User{}, errors.New("name must be 1 to 100 characters")
	}
	email, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	var result User
	err = database.InTx(ctx, pool, pgx.TxOptions{}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := q.LockUserAdministration(ctx); err != nil {
			return err
		}
		exists, err := q.HasAdministrator(ctx)
		if err != nil {
			return err
		}
		if exists {
			return ErrAlreadyBootstrapped
		}
		u, err := q.InsertUser(ctx, dbsql.InsertUserParams{Name: name, Email: email, PasswordHash: hash, Role: "ADMIN"})
		if err != nil {
			return err
		}
		if err := audit.UserCreated(ctx, q, pgtype.UUID{}, u, "ADMIN", "USER_BOOTSTRAPPED", "bootstrap-"+u.ID.String()); err != nil {
			return err
		}
		result = publicUser(u.ID, u.Name, u.Email, "ADMIN", u.IsActive, u.CreatedAt, u.UpdatedAt)
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return result, nil
}
