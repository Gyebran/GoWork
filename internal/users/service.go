package users

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Gyebran/GoWork/internal/audit"
	"github.com/Gyebran/GoWork/internal/auth"
	"github.com/Gyebran/GoWork/internal/platform/database"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/rbac"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Error string

func (e Error) Error() string { return string(e) }

const (
	Invalid          Error = "VALIDATION_ERROR"
	NotFound         Error = "USER_NOT_FOUND"
	SelfDeactivation Error = "SELF_DEACTIVATION"
	LastAdmin        Error = "LAST_ADMIN"
	ActiveOrders     Error = "USER_HAS_ACTIVE_WORK_ORDERS"
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool} }

type CreateInput struct{ Name, Email, Password, Role string }
type PatchInput struct {
	Name, Email, Password *string
	IsActive              *bool
}
type Filter struct {
	Page, Limit int
	Role        string
	Active      pgtype.Bool
}
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
type Page struct {
	Data []auth.User `json:"data"`
	Meta Meta        `json:"meta"`
}

func ID(raw string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' || id.Scan(raw) != nil {
		return id, Invalid
	}
	return id, nil
}
func validName(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) >= 1 && utf8.RuneCountInString(s) <= 100
}
func validRole(s string) bool { return s == "ADMIN" || s == "MANAGER" || s == "TECHNICIAN" }
func (s *Service) Authorize(ctx context.Context, actor pgtype.UUID, permission string) error {
	return rbac.Check(ctx, dbsql.New(s.pool), actor, permission)
}
func public(u dbsql.GetCurrentUserRow) auth.User {
	return auth.User{ID: u.ID.String(), Name: u.Name, Email: u.Email, Role: u.Role, IsActive: u.IsActive, CreatedAt: u.CreatedAt.Time.UTC(), UpdatedAt: u.UpdatedAt.Time.UTC()}
}
func (s *Service) Get(ctx context.Context, actor, id pgtype.UUID) (auth.User, error) {
	var out auth.User
	err := database.InTx(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := rbac.Check(ctx, q, actor, "user:read"); err != nil {
			return err
		}
		u, err := q.GetCurrentUser(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		out = public(u)
		return nil
	})
	return out, err
}
func (s *Service) List(ctx context.Context, actor pgtype.UUID, f Filter) (Page, error) {
	out := Page{Data: []auth.User{}, Meta: Meta{Page: f.Page, Limit: f.Limit}}
	if f.Page < 1 || f.Page > 100000 || f.Limit < 1 || f.Limit > 100 || (f.Role != "" && !validRole(f.Role)) {
		return out, Invalid
	}
	err := database.InTx(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := rbac.Check(ctx, q, actor, "user:read"); err != nil {
			return err
		}
		n, err := q.CountUsers(ctx, dbsql.CountUsersParams{RoleFilter: f.Role, ActiveFilter: f.Active})
		if err != nil {
			return err
		}
		rows, err := q.ListUsers(ctx, dbsql.ListUsersParams{RoleFilter: f.Role, ActiveFilter: f.Active, PageLimit: int32(f.Limit), PageOffset: int32((f.Page - 1) * f.Limit)})
		if err != nil {
			return err
		}
		for _, u := range rows {
			out.Data = append(out.Data, auth.User{ID: u.ID.String(), Name: u.Name, Email: u.Email, Role: u.Role, IsActive: u.IsActive, CreatedAt: u.CreatedAt.Time.UTC(), UpdatedAt: u.UpdatedAt.Time.UTC()})
		}
		out.Meta.Total = n
		out.Meta.TotalPages = (n + int64(f.Limit) - 1) / int64(f.Limit)
		return nil
	})
	return out, err
}
func (s *Service) Create(ctx context.Context, actor pgtype.UUID, in CreateInput, requestID string) (auth.User, error) {
	var out auth.User
	// Check before hashing, and again after acquiring the write lock.
	if err := s.Authorize(ctx, actor, "user:create"); err != nil {
		return out, err
	}
	in.Name = strings.TrimSpace(in.Name)
	email, err := auth.NormalizeEmail(in.Email)
	if err != nil || !validName(in.Name) || !validRole(in.Role) {
		return out, Invalid
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return out, Invalid
	}
	err = database.InTx(ctx, s.pool, pgx.TxOptions{}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := q.LockUserAdministration(ctx); err != nil {
			return err
		}
		if err := rbac.Check(ctx, q, actor, "user:create"); err != nil {
			return err
		}
		u, err := q.InsertUser(ctx, dbsql.InsertUserParams{Name: in.Name, Email: email, PasswordHash: hash, Role: in.Role})
		if err != nil {
			return err
		}
		if err := audit.UserCreated(ctx, q, actor, u, in.Role, "USER_CREATED", requestID); err != nil {
			return err
		}
		current, err := q.GetCurrentUser(ctx, u.ID)
		if err != nil {
			return err
		}
		out = public(current)
		return nil
	})
	return out, err
}

// GuardDeactivation is evaluated under the administration lock and target row lock.
func GuardDeactivation(self bool, role string, activeAdmins int64, hasOrders bool) error {
	if self {
		return SelfDeactivation
	}
	if role == "ADMIN" && activeAdmins <= 1 {
		return LastAdmin
	}
	if hasOrders {
		return ActiveOrders
	}
	return nil
}
func (s *Service) Patch(ctx context.Context, actor, id pgtype.UUID, in PatchInput, requestID string) (auth.User, error) {
	var out auth.User
	if err := s.Authorize(ctx, actor, "user:update"); err != nil {
		return out, err
	}
	if in.Name == nil && in.Email == nil && in.Password == nil && in.IsActive == nil {
		return out, Invalid
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if !validName(v) {
			return out, Invalid
		}
		in.Name = &v
	}
	if in.Email != nil {
		v, err := auth.NormalizeEmail(*in.Email)
		if err != nil {
			return out, Invalid
		}
		in.Email = &v
	}
	var hash string
	if in.Password != nil {
		var err error
		hash, err = auth.HashPassword(*in.Password)
		if err != nil {
			return out, Invalid
		}
	}
	err := database.InTx(ctx, s.pool, pgx.TxOptions{}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := q.LockUserAdministration(ctx); err != nil {
			return err
		}
		if err := rbac.Check(ctx, q, actor, "user:update"); err != nil {
			return err
		}
		old, err := q.LockUser(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		current, err := q.GetCurrentUser(ctx, id)
		if err != nil {
			return err
		}
		next := old
		if in.Name != nil {
			next.Name = *in.Name
		}
		if in.Email != nil {
			next.Email = *in.Email
		}
		if in.Password != nil {
			next.PasswordHash = hash
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		if old.IsActive && !next.IsActive {
			n, err := q.CountActiveAdmins(ctx)
			if err != nil {
				return err
			}
			assigned, err := q.UserHasActiveOrders(ctx, id)
			if err != nil {
				return err
			}
			if err := GuardDeactivation(actor == id, current.Role, n, assigned); err != nil {
				return err
			}
		}
		if next.Name == old.Name && next.Email == old.Email && next.IsActive == old.IsActive && in.Password == nil {
			out = public(current)
			return nil
		}
		next, err = q.UpdateUser(ctx, dbsql.UpdateUserParams{ID: id, Name: next.Name, Email: next.Email, PasswordHash: next.PasswordHash, IsActive: next.IsActive})
		if err != nil {
			return err
		}
		before := audit.UserSnapshot{ID: id.String(), Name: old.Name, Email: old.Email, Role: current.Role, IsActive: old.IsActive}
		after := struct {
			audit.UserSnapshot
			PasswordChanged bool `json:"password_changed,omitempty"`
		}{audit.UserSnapshot{ID: id.String(), Name: next.Name, Email: next.Email, Role: current.Role, IsActive: next.IsActive}, in.Password != nil}
		oldJSON, err := json.Marshal(before)
		if err != nil {
			return err
		}
		newJSON, err := json.Marshal(after)
		if err != nil {
			return err
		}
		if err := q.InsertUserUpdateAudit(ctx, dbsql.InsertUserUpdateAuditParams{ActorID: actor, EntityID: id, OldValue: oldJSON, NewValue: newJSON, RequestID: requestID}); err != nil {
			return err
		}
		updated, err := q.GetCurrentUser(ctx, id)
		if err != nil {
			return err
		}
		out = public(updated)
		return nil
	})
	return out, err
}
