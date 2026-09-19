package audit

import (
	"context"
	"encoding/json"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserSnapshot makes accidentally auditing a password hash impossible at this boundary.
type UserSnapshot struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`
}

func UserCreated(ctx context.Context, q *dbsql.Queries, actor pgtype.UUID, u dbsql.User, role, action, requestID string) error {
	snapshot, err := json.Marshal(UserSnapshot{u.ID.String(), u.Name, u.Email, role, u.IsActive})
	if err != nil {
		return err
	}
	return q.InsertAudit(ctx, dbsql.InsertAuditParams{ActorID: actor, Action: action, EntityType: "user", EntityID: u.ID, NewValue: snapshot, RequestID: requestID})
}
