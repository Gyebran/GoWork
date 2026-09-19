package auth

import (
	"context"
	"errors"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler { return &Handler{service, logger} }
func (h *Handler) Register(r chi.Router) {
	r.Method("POST", "/api/v1/auth/login", noStore(httpx.Endpoint(http.HandlerFunc(h.Login))))
	r.Method("GET", "/api/v1/auth/me", noStore(httpx.Endpoint(h.Authenticate(http.HandlerFunc(h.Me)))))
}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" {
		httpx.WriteError(w, r, 422, "VALIDATION_ERROR", "Query parameters are not supported")
		return
	}
	values, ok := httpx.DecodeStringObject(w, r, "email", "password")
	if !ok {
		return
	}
	email, err := NormalizeEmail(values["email"])
	password := values["password"]
	if err != nil || !utf8.ValidString(password) || len(password) < 1 || len(password) > 72 {
		httpx.WriteError(w, r, 422, "VALIDATION_ERROR", "Valid email and password of 1 to 72 UTF-8 bytes required")
		return
	}
	result, err := h.service.Login(r.Context(), email, password)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": result})
}

type currentUserKey struct{}

func CurrentUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(currentUserKey{}).(User)
	return u, ok
}
func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.handleError(w, r, ErrUnauthenticated)
			return
		}
		u, err := h.service.Current(r.Context(), parts[1])
		if err != nil {
			h.handleError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentUserKey{}, u)))
	})
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		httpx.WriteError(w, r, 422, "VALIDATION_ERROR", "Query parameters are not supported")
		return
	}
	u, ok := CurrentUser(r.Context())
	if !ok {
		h.handleError(w, r, ErrUnauthenticated)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": u})
}
func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	code, status, message := "INTERNAL_ERROR", 500, "An internal error occurred"
	switch {
	case errors.Is(err, ErrCredentials):
		code, status, message = "INVALID_CREDENTIALS", 401, "Invalid email or password"
	case errors.Is(err, ErrUnauthenticated):
		code, status, message = "UNAUTHENTICATED", 401, "Authentication required"
		w.Header().Set("WWW-Authenticate", "Bearer")
	case errors.Is(err, ErrUnavailable):
		code, status, message = "SERVICE_UNAVAILABLE", 503, "Authentication temporarily unavailable"
	}
	// Do not include submitted email, token, password, hash, or raw driver error.
	h.logger.WarnContext(r.Context(), "authentication_failed", "request_id", httpx.RequestID(r.Context()), "code", code)
	httpx.WriteError(w, r, status, code, message)
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
