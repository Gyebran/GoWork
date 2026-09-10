package httpx

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const RequestTimeout = 10 * time.Second

func NewRouter(logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(requestIDs, accessLog(logger), recoverPanic(logger))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, 404, "NOT_FOUND", "Route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		var allowed []string
		for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE"} {
			if r.Match(chi.NewRouteContext(), method, req.URL.Path) {
				allowed = append(allowed, method)
			}
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		writeError(w, req, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
	})
	r.Method("GET", "/health", requestTimeout(RequestTimeout)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})))
	return r
}
