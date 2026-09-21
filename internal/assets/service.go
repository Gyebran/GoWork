package assets

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
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
	Invalid    Error = "VALIDATION_ERROR"
	NotFound   Error = "ASSET_NOT_FOUND"
	InUse      Error = "ASSET_IN_USE"
	OpenOrders Error = "ASSET_HAS_OPEN_WORK_ORDERS"
)

type Asset struct {
	ID        string    `json:"id"`
	AssetCode string    `json:"asset_code"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Location  string    `json:"location"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
type Page struct {
	Data []Asset `json:"data"`
	Meta Meta    `json:"meta"`
}
type Filter struct {
	Page, Limit              int
	Status, Category, Search string
}
type Service struct{ pool *pgxpool.Pool }

func NewService(p *pgxpool.Pool) *Service { return &Service{p} }
func public(a dbsql.Asset) Asset {
	return Asset{a.ID.String(), a.AssetCode, a.Name, a.Category, a.Location, a.Status, a.CreatedAt.Time.UTC(), a.UpdatedAt.Time.UTC()}
}

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,49}$`)

func validText(s string, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) >= 1 && utf8.RuneCountInString(s) <= max
}
func validStatus(s string) bool {
	return s == "ACTIVE" || s == "INACTIVE" || s == "MAINTENANCE" || s == "RETIRED"
}
func ID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || id.Scan(s) != nil {
		return id, Invalid
	}
	return id, nil
}
func LiteralPattern(s string) string {
	if s == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s) + "%"
}
func (s *Service) Authorize(ctx context.Context, actor pgtype.UUID, permission string) error {
	return rbac.Check(ctx, dbsql.New(s.pool), actor, permission)
}
func scope(ctx context.Context, q *dbsql.Queries, actor pgtype.UUID) (bool, error) {
	u, err := q.GetCurrentUser(ctx, actor)
	if err != nil {
		return false, err
	}
	switch u.Role {
	case "ADMIN", "MANAGER":
		return false, nil
	case "TECHNICIAN":
		return true, nil
	default:
		return false, rbac.ErrForbidden
	}
}
func (s *Service) Get(ctx context.Context, actor, id pgtype.UUID) (Asset, error) {
	var out Asset
	err := database.InTx(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := rbac.Check(ctx, q, actor, "asset:read"); err != nil {
			return err
		}
		a, err := q.GetAsset(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		scoped, err := scope(ctx, q, actor)
		if err != nil {
			return err
		}
		if scoped {
			visible, err := q.AssetVisibleTo(ctx, dbsql.AssetVisibleToParams{AssetID: id, AssignedTo: actor})
			if err != nil {
				return err
			}
			if !visible {
				return rbac.ErrForbidden
			}
		}
		out = public(a)
		return nil
	})
	return out, err
}
func (s *Service) List(ctx context.Context, actor pgtype.UUID, f Filter) (Page, error) {
	out := Page{Data: []Asset{}, Meta: Meta{Page: f.Page, Limit: f.Limit}}
	if f.Page < 1 || f.Page > 100000 || f.Limit < 1 || f.Limit > 100 || (f.Status != "" && !validStatus(f.Status)) || (f.Category != "" && !validText(f.Category, 100)) || (f.Search != "" && !validText(f.Search, 100)) {
		return out, Invalid
	}
	err := database.InTx(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(_ pgx.Tx, q *dbsql.Queries) error {
		if err := rbac.Check(ctx, q, actor, "asset:read"); err != nil {
			return err
		}
		scoped, err := scope(ctx, q, actor)
		if err != nil {
			return err
		}
		n, err := q.CountAssets(ctx, dbsql.CountAssetsParams{StatusFilter: f.Status, CategoryFilter: f.Category, SearchPattern: LiteralPattern(f.Search), Scoped: scoped, ActorID: actor})
		if err != nil {
			return err
		}
		rows, err := q.ListAssets(ctx, dbsql.ListAssetsParams{StatusFilter: f.Status, CategoryFilter: f.Category, SearchPattern: LiteralPattern(f.Search), Scoped: scoped, ActorID: actor, PageLimit: int32(f.Limit), PageOffset: int32((f.Page - 1) * f.Limit)})
		if err != nil {
			return err
		}
		for _, a := range rows {
			out.Data = append(out.Data, public(a))
		}
		out.Meta.Total = n
		out.Meta.TotalPages = (n + int64(f.Limit) - 1) / int64(f.Limit)
		return nil
	})
	return out, err
}
func writeAudit(ctx context.Context, q *dbsql.Queries, actor, id pgtype.UUID, action, requestID string, before, after *Asset) error {
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
	return q.InsertAudit(ctx, dbsql.InsertAuditParams{ActorID: actor, EntityID: id, EntityType: "asset", Action: action, RequestID: requestID, OldValue: oldJSON, NewValue: newJSON})
}

// Mutations lock the actor FOR SHARE before rechecking permissions so deactivation
// cannot commit during an authorized write. No user administration lock is acquired.
func lockActor(ctx context.Context, tx pgx.Tx, q *dbsql.Queries, actor pgtype.UUID, permission string) error {
	if _, err := tx.Exec(ctx, "SELECT id FROM users WHERE id=$1 FOR SHARE", actor); err != nil {
		return err
	}
	return rbac.Check(ctx, q, actor, permission)
}
func (s *Service) Create(ctx context.Context, actor pgtype.UUID, values map[string]string, requestID string) (Asset, error) {
	var out Asset
	if err := s.Authorize(ctx, actor, "asset:create"); err != nil {
		return out, err
	}
	for k := range values {
		if k != "asset_code" && k != "name" && k != "category" && k != "location" && k != "status" {
			return out, Invalid
		}
	}
	status, ok := values["status"]
	if !ok {
		status = "ACTIVE"
	}
	name, category, location := strings.TrimSpace(values["name"]), strings.TrimSpace(values["category"]), strings.TrimSpace(values["location"])
	if !codePattern.MatchString(values["asset_code"]) || !validText(name, 150) || !validText(category, 100) || !validText(location, 200) || !validStatus(status) {
		return out, Invalid
	}
	err := database.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx, q *dbsql.Queries) error {
		if err := lockActor(ctx, tx, q, actor, "asset:create"); err != nil {
			return err
		}
		a, err := q.InsertAsset(ctx, dbsql.InsertAssetParams{AssetCode: values["asset_code"], Name: name, Category: category, Location: location, Status: status})
		if err != nil {
			return err
		}
		out = public(a)
		return writeAudit(ctx, q, actor, a.ID, "ASSET_CREATED", requestID, nil, &out)
	})
	return out, err
}
func (s *Service) Patch(ctx context.Context, actor, id pgtype.UUID, values map[string]string, requestID string) (Asset, error) {
	var out Asset
	if err := s.Authorize(ctx, actor, "asset:update"); err != nil {
		return out, err
	}
	if len(values) == 0 {
		return out, Invalid
	}
	clean := map[string]string{}
	for k, v := range values {
		switch k {
		case "name", "category", "location":
			v = strings.TrimSpace(v)
			max := 100
			if k == "name" {
				max = 150
			}
			if k == "location" {
				max = 200
			}
			if !validText(v, max) {
				return out, Invalid
			}
		case "status":
			if !validStatus(v) {
				return out, Invalid
			}
		default:
			return out, Invalid
		}
		clean[k] = v
	}
	err := database.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx, q *dbsql.Queries) error {
		if err := lockActor(ctx, tx, q, actor, "asset:update"); err != nil {
			return err
		}
		old, err := q.LockAsset(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		next := old
		for k, v := range clean {
			switch k {
			case "name":
				next.Name = v
			case "category":
				next.Category = v
			case "location":
				next.Location = v
			case "status":
				next.Status = v
			}
		}
		if next == old {
			out = public(old)
			return nil
		}
		if next.Status != old.Status && (next.Status == "INACTIVE" || next.Status == "RETIRED") {
			has, err := q.AssetHasOpenOrders(ctx, id)
			if err != nil {
				return err
			}
			if has {
				return OpenOrders
			}
		}
		next, err = q.UpdateAsset(ctx, dbsql.UpdateAssetParams{ID: id, Name: next.Name, Category: next.Category, Location: next.Location, Status: next.Status})
		if err != nil {
			return err
		}
		before := public(old)
		out = public(next)
		return writeAudit(ctx, q, actor, id, "ASSET_UPDATED", requestID, &before, &out)
	})
	return out, err
}
func (s *Service) Delete(ctx context.Context, actor, id pgtype.UUID, requestID string) error {
	return database.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx, q *dbsql.Queries) error {
		if err := lockActor(ctx, tx, q, actor, "asset:delete"); err != nil {
			return err
		}
		a, err := q.LockAsset(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		used, err := q.AssetHasOrders(ctx, id)
		if err != nil {
			return err
		}
		if used {
			return InUse
		}
		if err := q.DeleteAsset(ctx, id); err != nil {
			return err
		}
		before := public(a)
		return writeAudit(ctx, q, actor, id, "ASSET_DELETED", requestID, &before, nil)
	})
}
