package httpx

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const RequestTimeout = 10 * time.Second

func NewRouter(logger *slog.Logger, readiness ...*Readiness) http.Handler {
	var ready *Readiness
	if len(readiness) > 0 {
		ready = readiness[0]
	}
	return NewRouterWithRoutes(logger, ready, nil)
}

func Endpoint(handler http.Handler) http.Handler { return requestTimeout(RequestTimeout)(handler) }

func NewRouterWithRoutes(logger *slog.Logger, ready *Readiness, register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Use(requestIDs, accessLog(logger), recoverPanic(logger))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, 404, "NOT_FOUND", "Route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		var allowed []string
		for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE"} {
			if r.Match(chi.NewRouteContext(), method, req.URL.Path) {
				allowed = append(allowed, method)
			}
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		WriteError(w, req, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
	})
	r.Method("GET", "/health", requestTimeout(RequestTimeout)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, 200, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})))
	if ready != nil {
		r.Method("GET", "/ready", requestTimeout(RequestTimeout)(ready))
	}
	if register != nil {
		register(r)
	}
	return r
}
