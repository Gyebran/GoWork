package workorders

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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
	Invalid           Error = "VALIDATION_ERROR"
	NotFound          Error = "WORK_ORDER_NOT_FOUND"
	AssetNotFound     Error = "ASSET_NOT_FOUND"
	AssetUnavailable  Error = "ASSET_NOT_AVAILABLE"
	InvalidAssignee   Error = "INVALID_ASSIGNEE"
	Terminal          Error = "WORK_ORDER_TERMINAL"
	InvalidTransition Error = "INVALID_STATUS_TRANSITION"
	InvalidAssignment Error = "INVALID_ASSIGNMENT_STATE"
	AlreadyAssigned   Error = "ALREADY_ASSIGNED"
)

type Order struct {
	ID          string     `json:"id"`
	Number      string     `json:"work_order_number"`
	AssetID     string     `json:"asset_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    string     `json:"priority"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by"`
	AssignedTo  *string    `json:"assigned_to"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
type Page struct {
	Data []Order `json:"data"`
	Meta Meta    `json:"meta"`
}
type Filter struct {
	Page, Limit      int
	Status, Priority string
	Asset, Assignee  pgtype.UUID
}
type Service struct{ pool *pgxpool.Pool }

func NewService(p *pgxpool.Pool) *Service { return &Service{p} }
func public(w dbsql.WorkOrder) Order {
	o := Order{ID: w.ID.String(), Number: w.WorkOrderNumber, AssetID: w.AssetID.String(), Title: w.Title, Description: w.Description, Priority: w.Priority, Status: w.Status, CreatedBy: w.CreatedBy.String(), CreatedAt: w.CreatedAt.Time.UTC(), UpdatedAt: w.UpdatedAt.Time.UTC()}
	if w.AssignedTo.Valid {
		v := w.AssignedTo.String()
		o.AssignedTo = &v
	}
	if w.CompletedAt.Valid {
		v := w.CompletedAt.Time.UTC()
		o.CompletedAt = &v
	}
	return o
}
func ID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || id.Scan(s) != nil {
		return id, Invalid
	}
	return id, nil
}
func validText(s string, min, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) >= min && utf8.RuneCountInString(s) <= max
}
func validPriority(s string) bool {
	return s == "LOW" || s == "MEDIUM" || s == "HIGH" || s == "CRITICAL"
}
func validStatus(s string) bool {
	return s == "OPEN" || s == "ASSIGNED" || s == "IN_PROGRESS" || s == "COMPLETED" || s == "CANCELLED"
}
func Transition(from, to string) error {
	if from == "COMPLETED" || from == "CANCELLED" {
		return Terminal
	}
	if (to == "CANCELLED" && (from == "OPEN" || from == "ASSIGNED" || from == "IN_PROGRESS")) || (from == "ASSIGNED" && to == "IN_PROGRESS") || (from == "IN_PROGRESS" && to == "COMPLETED") {
		return nil
	}
	return InvalidTransition
}
func (s *Service) Authorize(ctx context.Context, a pgtype.UUID, p string) error {
	return rbac.Check(ctx, dbsql.New(s.pool), a, p)
}
func currentRole(ctx context.Context, q *dbsql.Queries, a pgtype.UUID) (string, error) {
	u, err := q.GetCurrentUser(ctx, a)
	return u.Role, err
}
func lockActor(ctx context.Context, q *dbsql.Queries, a pgtype.UUID, p string) error {
	_, err := q.ShareUser(ctx, a)
	if errors.Is(err, pgx.ErrNoRows) {
		return rbac.ErrInactive
	}
	if err != nil {
		return err
	}
	return rbac.Check(ctx, q, a, p)
}
func visible(role string, actor pgtype.UUID, w dbsql.WorkOrder) bool {
	return rbac.AssignedScope(role, actor.String(), w.AssignedTo.String())
}
func (s *Service) Get(ctx context.Context, a, id pgtype.UUID) (Order, error) {
	var out Order
	err := database.InTx(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := rbac.Check(ctx, q, a, "work_order:read"); err != nil {
			return err
		}
		w, err := q.GetWorkOrder(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		role, err := currentRole(ctx, q, a)
		if err != nil {
			return err
		}
		if !visible(role, a, w) {
			return rbac.ErrForbidden
		}
		out = public(w)
		return nil
	})
	return out, err
}
func (s *Service) List(ctx context.Context, a pgtype.UUID, f Filter) (Page, error) {
	out := Page{Data: []Order{}, Meta: Meta{Page: f.Page, Limit: f.Limit}}
	if f.Page < 1 || f.Page > 100000 || f.Limit < 1 || f.Limit > 100 || (f.Status != "" && !validStatus(f.Status)) || (f.Priority != "" && !validPriority(f.Priority)) {
		return out, Invalid
	}
	err := database.InTx(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := rbac.Check(ctx, q, a, "work_order:read"); err != nil {
			return err
		}
		role, err := currentRole(ctx, q, a)
		if err != nil {
			return err
		}
		filter := ""
		if f.Assignee.Valid {
			filter = f.Assignee.String()
		}
		if !rbac.AssignmentFilterAllowed(role, a.String(), filter) {
			return rbac.ErrForbidden
		}
		n, err := q.CountWorkOrders(ctx, dbsql.CountWorkOrdersParams{StatusFilter: f.Status, PriorityFilter: f.Priority, AssetFilter: f.Asset, AssigneeFilter: f.Assignee, Scoped: role == "TECHNICIAN", ActorID: a})
		if err != nil {
			return err
		}
		rows, err := q.ListWorkOrders(ctx, dbsql.ListWorkOrdersParams{StatusFilter: f.Status, PriorityFilter: f.Priority, AssetFilter: f.Asset, AssigneeFilter: f.Assignee, Scoped: role == "TECHNICIAN", ActorID: a, PageLimit: int32(f.Limit), PageOffset: int32((f.Page - 1) * f.Limit)})
		if err != nil {
			return err
		}
		for _, w := range rows {
			out.Data = append(out.Data, public(w))
		}
		out.Meta.Total = n
		out.Meta.TotalPages = (n + int64(f.Limit) - 1) / int64(f.Limit)
		return nil
	})
	return out, err
}
func audit(ctx context.Context, q *dbsql.Queries, a, id pgtype.UUID, action, request string, before, after *Order) error {
	var oldJSON, newJSON []byte
	var err error
	if before != nil {
		oldJSON, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		newJSON, err = json.Marshal(after)
		if err != nil {
			return err
		}
	}
	return q.InsertAudit(ctx, dbsql.InsertAuditParams{ActorID: a, EntityID: id, Action: action, EntityType: "work_order", RequestID: request, OldValue: oldJSON, NewValue: newJSON})
}
func (s *Service) Create(ctx context.Context, a pgtype.UUID, v map[string]string, request string) (Order, error) {
	var out Order
	if err := s.Authorize(ctx, a, "work_order:create"); err != nil {
		return out, err
	}
	for k := range v {
		if k != "asset_id" && k != "title" && k != "description" && k != "priority" {
			return out, Invalid
		}
	}
	asset, err := ID(v["asset_id"])
	title := strings.TrimSpace(v["title"])
	priority, ok := v["priority"]
	if !ok {
		priority = "MEDIUM"
	}
	if err != nil || !validText(title, 1, 200) || !validText(v["description"], 0, 5000) || !validPriority(priority) {
		return out, Invalid
	}
	err = database.InTx(ctx, s.pool, pgx.TxOptions{}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := lockActor(ctx, q, a, "work_order:create"); err != nil {
			return err
		}
		asset, err := q.ShareAsset(ctx, asset)
		if errors.Is(err, pgx.ErrNoRows) {
			return AssetNotFound
		}
		if err != nil {
			return err
		}
		if asset.Status != "ACTIVE" && asset.Status != "MAINTENANCE" {
			return AssetUnavailable
		}
		w, err := q.InsertWorkOrder(ctx, dbsql.InsertWorkOrderParams{AssetID: asset.ID, Title: title, Description: v["description"], Priority: priority, CreatedBy: a})
		if err != nil {
			return err
		}
		out = public(w)
		return audit(ctx, q, a, w.ID, "WORK_ORDER_CREATED", request, nil, &out)
	})
	return out, err
}

// mutate serializes ownership/lifecycle checks with every update to the order.
func (s *Service) mutate(ctx context.Context, a, id pgtype.UUID, permission, request string, change func(*dbsql.Queries, dbsql.WorkOrder) (dbsql.WorkOrder, []string, error), targets ...pgtype.UUID) (Order, error) {
	var out Order
	err := database.InTx(ctx, s.pool, pgx.TxOptions{}, func(_ pgx.Tx, q *dbsql.Queries) error {
		ids := append([]pgtype.UUID{a}, targets...)
		sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
		for _, id := range ids {
			if _, err := q.ShareUser(ctx, id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if err := rbac.Check(ctx, q, a, permission); err != nil {
			return err
		}
		old, err := q.LockWorkOrder(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		role, err := currentRole(ctx, q, a)
		if err != nil {
			return err
		}
		if !visible(role, a, old) {
			return rbac.ErrForbidden
		}
		if old.Status == "COMPLETED" || old.Status == "CANCELLED" {
			return Terminal
		}
		next, actions, err := change(q, old)
		if err != nil {
			return err
		}
		if len(actions) == 0 {
			out = public(old)
			return nil
		}
		next, err = q.UpdateWorkOrder(ctx, dbsql.UpdateWorkOrderParams{ID: id, Title: next.Title, Description: next.Description, Priority: next.Priority, Status: next.Status, AssignedTo: next.AssignedTo})
		if err != nil {
			return err
		}
		before := public(old)
		out = public(next)
		for _, action := range actions {
			if err := audit(ctx, q, a, id, action, request, &before, &out); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
func (s *Service) Patch(ctx context.Context, a, id pgtype.UUID, v map[string]string, request string) (Order, error) {
	if err := s.Authorize(ctx, a, "work_order:update"); err != nil {
		return Order{}, err
	}
	if len(v) == 0 {
		return Order{}, Invalid
	}
	clean := map[string]string{}
	for k, x := range v {
		switch k {
		case "title":
			x = strings.TrimSpace(x)
			if !validText(x, 1, 200) {
				return Order{}, Invalid
			}
		case "description":
			if !validText(x, 0, 5000) {
				return Order{}, Invalid
			}
		case "priority":
			if !validPriority(x) {
				return Order{}, Invalid
			}
		default:
			return Order{}, Invalid
		}
		clean[k] = x
	}
	return s.mutate(ctx, a, id, "work_order:update", request, func(_ *dbsql.Queries, w dbsql.WorkOrder) (dbsql.WorkOrder, []string, error) {
		old := w
		for k, x := range clean {
			switch k {
			case "title":
				w.Title = x
			case "description":
				w.Description = x
			case "priority":
				w.Priority = x
			}
		}
		if w == old {
			return w, nil, nil
		}
		return w, []string{"WORK_ORDER_UPDATED"}, nil
	})
}
func (s *Service) Assign(ctx context.Context, a, id, target pgtype.UUID, request string) (Order, error) {
	return s.mutate(ctx, a, id, "work_order:assign", request, func(q *dbsql.Queries, w dbsql.WorkOrder) (dbsql.WorkOrder, []string, error) {
		if w.Status != "OPEN" && w.Status != "ASSIGNED" {
			return w, nil, InvalidAssignment
		}
		if w.AssignedTo.Valid && w.AssignedTo == target {
			return w, nil, AlreadyAssigned
		}
		u, err := q.ShareUser(ctx, target)
		if errors.Is(err, pgx.ErrNoRows) {
			return w, nil, InvalidAssignee
		}
		if err != nil {
			return w, nil, err
		}
		current, err := q.GetCurrentUser(ctx, u.ID)
		if err != nil {
			return w, nil, err
		}
		if !u.IsActive || current.Role != "TECHNICIAN" {
			return w, nil, InvalidAssignee
		}
		actions := []string{"WORK_ORDER_ASSIGNED"}
		if w.Status == "OPEN" {
			actions = append(actions, "WORK_ORDER_STATUS_CHANGED")
		}
		w.Status = "ASSIGNED"
		w.AssignedTo = target
		return w, actions, nil
	}, target)
}
func StatusPermission(target string) string {
	if target == "CANCELLED" {
		return "work_order:cancel"
	}
	return "work_order:status:update"
}
func (s *Service) Status(ctx context.Context, a, id pgtype.UUID, target, request string) (Order, error) {
	if err := s.Authorize(ctx, a, StatusPermission(target)); err != nil {
		return Order{}, err
	}
	if target != "IN_PROGRESS" && target != "COMPLETED" && target != "CANCELLED" {
		return Order{}, Invalid
	}
	return s.mutate(ctx, a, id, StatusPermission(target), request, func(_ *dbsql.Queries, w dbsql.WorkOrder) (dbsql.WorkOrder, []string, error) {
		if err := Transition(w.Status, target); err != nil {
			return w, nil, err
		}
		w.Status = target
		return w, []string{"WORK_ORDER_STATUS_CHANGED"}, nil
	})
}
