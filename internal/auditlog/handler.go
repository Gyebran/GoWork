package auditlog

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Gyebran/GoWork/internal/auth"
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
	r.Method("GET", "/api/v1/audit-logs", httpx.Endpoint(h.authentication.Authenticate(http.HandlerFunc(h.list))))
}
func actor(r *http.Request) pgtype.UUID {
	u, _ := auth.CurrentUser(r.Context())
	id, _ := ID(u.ID)
	return id
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	code, status, message := "INTERNAL_ERROR", 500, "An internal error occurred"
	var domain Error
	var pgerr *pgconn.PgError
	var neterr net.Error
	switch {
	case errors.Is(err, rbac.ErrInactive):
		code, status, message = "UNAUTHENTICATED", 401, "Authentication required"
		w.Header().Set("WWW-Authenticate", "Bearer")
	case errors.Is(err, rbac.ErrForbidden):
		code, status, message = "FORBIDDEN", 403, "Permission denied"
	case errors.As(err, &domain):
		code = string(domain)
		status = 422
		message = "Request validation failed"
	case errors.As(err, &pgerr):
		if pgerr.Code == "40001" || pgerr.Code == "40P01" {
			code, status, message = "CONCURRENT_MODIFICATION", 409, "Concurrent modification; reload current state"
		} else if len(pgerr.Code) >= 2 && (pgerr.Code[:2] == "08" || pgerr.Code == "57P01" || pgerr.Code == "57P02" || pgerr.Code == "57P03" || pgerr.Code == "57014") {
			code, status, message = "SERVICE_UNAVAILABLE", 503, "Service temporarily unavailable"
		}
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &neterr):
		code, status, message = "SERVICE_UNAVAILABLE", 503, "Service temporarily unavailable"
	}
	h.logger.WarnContext(r.Context(), "audit_request_failed", "request_id", httpx.RequestID(r.Context()), "code", code)
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
		case "action":
			if !validAction(v[0]) {
				return f, Invalid
			}
			f.Action = v[0]
		case "entity_type":
			if !validType(v[0]) {
				return f, Invalid
			}
			f.EntityType = v[0]
		case "actor_id", "entity_id":
			id, err := ID(v[0])
			if err != nil {
				return f, err
			}
			if k == "actor_id" {
				f.Actor = id
			} else {
				f.Entity = id
			}
		default:
			return f, Invalid
		}
	}
	if f.Page < 1 || f.Page > 100000 || f.Limit < 1 || f.Limit > 100 || (f.Entity.Valid && f.EntityType == "") {
		return f, Invalid
	}
	return f, nil
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Authorize(r.Context(), actor(r)); err != nil {
		h.fail(w, r, err)
		return
	}
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
