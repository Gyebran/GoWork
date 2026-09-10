package httpx

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type requestIDKey struct{}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func requestIDs(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if len(r.Header.Values("X-Request-ID")) != 1 || !validRequestID.MatchString(id) {
			var b [16]byte
			if _, err := rand.Read(b[:]); err != nil {
				panic("request ID generation failed")
			}
			b[6] = (b[6] & 0x0f) | 0x40
			b[8] = (b[8] & 0x3f) | 0x80
			id = fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			// Never log raw paths, query values, headers or bodies.
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "[unmatched]"
			}
			logger.InfoContext(r.Context(), "http_request", "request_id", RequestID(r.Context()), "method", r.Method, "route", route, "status", status, "duration_ms", time.Since(started).Milliseconds())
		})
	}
}

func recoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					if p == http.ErrAbortHandler {
						panic(p)
					}
					// Panic values may contain credentials. Keep them out of operational logs.
					logger.ErrorContext(r.Context(), "request_panic", "request_id", RequestID(r.Context()))
					writeError(w, r, 500, "INTERNAL_ERROR", "An internal error occurred")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutHandler buffers output and cancels the request context. The JSON writer
// replaces its default HTML content type on timeout. Handlers must honor context;
// Go cannot forcibly terminate a goroutine that ignores cancellation.
func requestTimeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := json.Marshal(errorBody{Error: apiError{Code: "SERVICE_UNAVAILABLE", Message: "Request deadline exceeded"}, RequestID: RequestID(r.Context())})
			http.TimeoutHandler(next, duration, string(body)+"\n").ServeHTTP(jsonTimeoutWriter{w}, r)
		})
	}
}

type jsonTimeoutWriter struct{ http.ResponseWriter }

func (w jsonTimeoutWriter) WriteHeader(status int) {
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Content-Type", "application/json")
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w jsonTimeoutWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
