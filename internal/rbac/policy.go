// Package rbac combines explicit database permissions with resource scope policies.
package rbac

import (
	"context"
	"errors"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrForbidden = errors.New("permission denied")
var ErrInactive = errors.New("active authentication required")

// Check uses current database state, never a role claim or an administrator bypass.
func Check(ctx context.Context, q *dbsql.Queries, actor pgtype.UUID, permission string) error {
	u, err := q.GetCurrentUser(ctx, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInactive
	}
	if err != nil {
		return err
	}
	if !u.IsActive {
		return ErrInactive
	}
	allowed, err := q.UserHasPermission(ctx, dbsql.UserHasPermissionParams{ID: actor, Code: permission})
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// AssignedScope must be combined with a successful coarse permission check.
// SQL list/count queries must apply the equivalent predicate before pagination.
func AssignedScope(role, actor, assignee string) bool {
	switch role {
	case "ADMIN", "MANAGER":
		return true
	case "TECHNICIAN":
		return actor != "" && actor == assignee
	default:
		return false
	}
}
func AssignmentFilterAllowed(role, actor, filter string) bool {
	if role == "TECHNICIAN" {
		return filter == "" || filter == actor
	}
	return role == "ADMIN" || role == "MANAGER"
}
