package workorders

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Gyebran/GoWork/internal/auth"
	"github.com/Gyebran/GoWork/internal/platform/database"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/Gyebran/GoWork/internal/rbac"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct {
	service        *Service
	authentication *auth.Handler
	logger         *slog.Logger
}

func NewHandler(s *Service, a *auth.Handler, l *slog.Logger) *Handler { return &Handler{s, a, l} }
func (h *Handler) Register(r chi.Router) {
	for _, route := range []struct {
		method, path, permission string
		fn                       http.HandlerFunc
	}{
		{"GET", "/api/v1/work-orders", "work_order:read", h.list},
		{"GET", "/api/v1/work-orders/{id}", "work_order:read", h.get},
		{"POST", "/api/v1/work-orders", "work_order:create", h.create},
		{"PATCH", "/api/v1/work-orders/{id}", "work_order:update", h.patch},
		{"PATCH", "/api/v1/work-orders/{id}/assign", "work_order:assign", h.assign},
	} {
		r.Method(route.method, route.path, httpx.Endpoint(h.authentication.Authenticate(h.require(route.permission, route.fn))))
	}
	r.Method("PATCH", "/api/v1/work-orders/{id}/status", httpx.Endpoint(h.authentication.Authenticate(http.HandlerFunc(h.status))))
}
func actor(r *http.Request) pgtype.UUID {
	u, _ := auth.CurrentUser(r.Context())
	id, _ := ID(u.ID)
	return id
}
func (h *Handler) require(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h.service.Authorize(r.Context(), actor(r), permission); err != nil {
			h.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	code, status, message := "INTERNAL_ERROR", 500, "An internal error occurred"
	var domain Error
	var pgerr *pgconn.PgError
	var neterr net.Error
	switch {
	case errors.Is(err, database.ErrWriteOutcomeUnknown):
		code, status, message = "WRITE_OUTCOME_UNKNOWN", 503, "Write outcome is unknown; check current state before retrying"
	case errors.Is(err, rbac.ErrInactive):
		code, status, message = "UNAUTHENTICATED", 401, "Authentication required"
		w.Header().Set("WWW-Authenticate", "Bearer")
	case errors.Is(err, rbac.ErrForbidden):
		code, status, message = "FORBIDDEN", 403, "Permission denied"
	case errors.As(err, &domain):
		code = string(domain)
		status = 409
		message = "Work order update conflicts with current state"
		if domain == Invalid {
			status = 422
			message = "Request validation failed"
		}
		if domain == NotFound || domain == AssetNotFound {
			status = 404
			message = "Resource not found"
		}
	case errors.As(err, &pgerr):
		if pgerr.Code == "40001" || pgerr.Code == "40P01" {
			code, status, message = "CONCURRENT_MODIFICATION", 409, "Concurrent modification; reload current state"
		} else if len(pgerr.Code) >= 2 && (pgerr.Code[:2] == "08" || pgerr.Code == "57P01" || pgerr.Code == "57P02" || pgerr.Code == "57P03" || pgerr.Code == "57014") {
			code, status, message = "SERVICE_UNAVAILABLE", 503, "Service temporarily unavailable"
		}
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &neterr):
		code, status, message = "SERVICE_UNAVAILABLE", 503, "Service temporarily unavailable"
	}
	h.logger.WarnContext(r.Context(), "work_order_request_failed", "request_id", httpx.RequestID(r.Context()), "code", code)
	httpx.WriteError(w, r, status, code, message)
}
func ParseFilter(raw string) (Filter, error) {
	f := Filter{Page: 1, Limit: 20}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, Invalid
	}
	for k, v := range q {
		if len(v) != 1 {
			return f, Invalid
		}
		switch k {
		case "page", "limit":
			if v[0] == "" {
				return f, Invalid
			}
			for _, c := range v[0] {
				if c < '0' || c > '9' {
					return f, Invalid
				}
			}
			n, err := strconv.Atoi(v[0])
			if err != nil {
				return f, Invalid
			}
			if k == "page" {
				f.Page = n
			} else {
				f.Limit = n
			}
		case "status":
			if !validStatus(v[0]) {
				return f, Invalid
			}
			f.Status = v[0]
		case "priority":
			if !validPriority(v[0]) {
				return f, Invalid
			}
			f.Priority = v[0]
		case "asset_id", "assigned_to":
			id, err := ID(v[0])
			if err != nil {
				return f, err
			}
			if k == "asset_id" {
				f.Asset = id
			} else {
				f.Assignee = id
			}
		default:
			return f, Invalid
		}
	}
	if f.Page < 1 || f.Page > 100000 || f.Limit < 1 || f.Limit > 100 {
		return f, Invalid
	}
	return f, nil
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	f, err := ParseFilter(r.URL.RawQuery)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out, err := h.service.List(r.Context(), actor(r), f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, out)
}
func (h *Handler) target(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	if r.URL.RawQuery != "" {
		h.fail(w, r, Invalid)
		return pgtype.UUID{}, false
	}
	id, err := ID(chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return id, false
	}
	return id, true
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.target(w, r)
	if !ok {
		return
	}
	out, err := h.service.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": out})
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		h.fail(w, r, Invalid)
		return
	}
	v, ok := httpx.DecodeStringObject(w, r, "asset_id", "title", "description", "priority")
	if !ok {
		return
	}
	out, err := h.service.Create(r.Context(), actor(r), v, httpx.RequestID(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/work-orders/"+out.ID)
	httpx.WriteJSON(w, 201, map[string]any{"data": out})
}
func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := h.target(w, r)
	if !ok {
		return
	}
	v, ok := httpx.DecodeStringObject(w, r, "title", "description", "priority")
	if !ok {
		return
	}
	out, err := h.service.Patch(r.Context(), actor(r), id, v, httpx.RequestID(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": out})
}
func (h *Handler) assign(w http.ResponseWriter, r *http.Request) {
	id, ok := h.target(w, r)
	if !ok {
		return
	}
	v, ok := httpx.DecodeStringObject(w, r, "assigned_to")
	if !ok {
		return
	}
	target, err := ID(v["assigned_to"])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out, err := h.service.Assign(r.Context(), actor(r), id, target, httpx.RequestID(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": out})
}
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	// Target-dependent permission is resolved before loading the resource.
	v, ok := httpx.DecodeStringObject(w, r, "status")
	if !ok {
		return
	}
	if err := h.service.Authorize(r.Context(), actor(r), StatusPermission(v["status"])); err != nil {
		h.fail(w, r, err)
		return
	}
	id, ok := h.target(w, r)
	if !ok {
		return
	}
	out, err := h.service.Status(r.Context(), actor(r), id, v["status"], httpx.RequestID(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": out})
}
