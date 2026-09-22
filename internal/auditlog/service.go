// Package auditlog implements the read API separately from the transaction-bound
// audit writer, avoiding a dependency cycle with authentication/bootstrap.
package auditlog

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/rbac"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Error string

func (e Error) Error() string { return string(e) }

const Invalid Error = "VALIDATION_ERROR"

var errSnapshot = errors.New("invalid audit snapshot")

type Filter struct {
	Page, Limit        int
	Actor, Entity      pgtype.UUID
	Action, EntityType string
}
type Entry struct {
	ID         string          `json:"id"`
	ActorID    *string         `json:"actor_id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	OldValue   json.RawMessage `json:"old_value"`
	NewValue   json.RawMessage `json:"new_value"`
	RequestID  string          `json:"request_id"`
	CreatedAt  time.Time       `json:"created_at"`
}
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
type Page struct {
	Data []Entry `json:"data"`
	Meta Meta    `json:"meta"`
}
type Service struct{ pool *pgxpool.Pool }

func NewService(p *pgxpool.Pool) *Service { return &Service{p} }
func ID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || id.Scan(s) != nil {
		return id, Invalid
	}
	return id, nil
}
func validType(s string) bool { return s == "user" || s == "asset" || s == "work_order" }
func validAction(s string) bool {
	switch s {
	case "USER_BOOTSTRAPPED", "USER_CREATED", "USER_UPDATED", "ASSET_CREATED", "ASSET_UPDATED", "ASSET_DELETED", "WORK_ORDER_CREATED", "WORK_ORDER_ASSIGNED", "WORK_ORDER_STATUS_CHANGED", "WORK_ORDER_UPDATED":
		return true
	}
	return false
}
func (s *Service) Authorize(ctx context.Context, a pgtype.UUID) error {
	return rbac.Check(ctx, dbsql.New(s.pool), a, "audit:read")
}

// Sanitize projects only known public fields with their expected primitive types.
// Unknown legacy fields cannot leak through raw JSON, including nested secrets.
func Sanitize(kind string, raw []byte) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var source map[string]json.RawMessage
	if json.Unmarshal(raw, &source) != nil || source == nil {
		return nil, errSnapshot
	}
	fields := map[string]string{"id": "string"}
	switch kind {
	case "user":
		for _, k := range []string{"name", "email", "role"} {
			fields[k] = "string"
		}
		fields["is_active"] = "bool"
		fields["password_changed"] = "bool"
	case "asset":
		for _, k := range []string{"asset_code", "name", "category", "location", "status", "created_at", "updated_at"} {
			fields[k] = "string"
		}
	case "work_order":
		for _, k := range []string{"work_order_number", "asset_id", "title", "description", "priority", "status", "created_by", "created_at", "updated_at"} {
			fields[k] = "string"
		}
		fields["assigned_to"] = "nullable"
		fields["completed_at"] = "nullable"
	default:
		return nil, errSnapshot
	}
	out := map[string]json.RawMessage{}
	for k, t := range fields {
		v, ok := source[k]
		if !ok {
			continue
		}
		if string(v) == "null" {
			if t != "nullable" {
				return nil, errSnapshot
			}
			out[k] = v
			continue
		}
		var target any
		if t == "bool" {
			target = new(bool)
		} else {
			target = new(string)
		}
		if json.Unmarshal(v, target) != nil {
			return nil, errSnapshot
		}
		out[k] = v
	}
	b, err := json.Marshal(out)
	return b, err
}
func (s *Service) List(ctx context.Context, a pgtype.UUID, f Filter) (Page, error) {
	out := Page{Data: []Entry{}, Meta: Meta{Page: f.Page, Limit: f.Limit}}
	if f.Page < 1 || f.Page > 100000 || f.Limit < 1 || f.Limit > 100 || (f.Action != "" && !validAction(f.Action)) || (f.EntityType != "" && !validType(f.EntityType)) || (f.Entity.Valid && f.EntityType == "") {
		return out, Invalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := dbsql.New(tx)
	if err = rbac.Check(ctx, q, a, "audit:read"); err != nil {
		return out, err
	}
	total, err := q.CountAuditLogs(ctx, dbsql.CountAuditLogsParams{ActorFilter: f.Actor, ActionFilter: f.Action, TypeFilter: f.EntityType, EntityFilter: f.Entity})
	if err != nil {
		return out, err
	}
	rows, err := q.ListAuditLogs(ctx, dbsql.ListAuditLogsParams{ActorFilter: f.Actor, ActionFilter: f.Action, TypeFilter: f.EntityType, EntityFilter: f.Entity, PageLimit: int32(f.Limit), PageOffset: int32((f.Page - 1) * f.Limit)})
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		old, err := Sanitize(r.EntityType, r.OldValue)
		if err != nil {
			return out, err
		}
		next, err := Sanitize(r.EntityType, r.NewValue)
		if err != nil {
			return out, err
		}
		entry := Entry{ID: r.ID.String(), Action: r.Action, EntityType: r.EntityType, EntityID: r.EntityID.String(), OldValue: old, NewValue: next, RequestID: r.RequestID, CreatedAt: r.CreatedAt.Time.UTC()}
		if r.ActorID.Valid {
			v := r.ActorID.String()
			entry.ActorID = &v
		}
		out.Data = append(out.Data, entry)
	}
	out.Meta.Total = total
	out.Meta.TotalPages = (total + int64(f.Limit) - 1) / int64(f.Limit)
	// Read-only transaction errors never claim an uncertain business write.
	if err = tx.Commit(ctx); err != nil {
		return Page{}, err
	}
	return out, nil
}
