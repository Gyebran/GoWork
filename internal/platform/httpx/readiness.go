package httpx

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

type Readiness struct {
	check    func(context.Context) error
	stopping atomic.Bool
}

func NewReadiness(check func(context.Context) error) *Readiness { return &Readiness{check: check} }
func (r *Readiness) Stop()                                      { r.stopping.Store(true) }
func (r *Readiness) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r == nil || r.check == nil || r.stopping.Load() {
		writeError(w, req, 503, "SERVICE_UNAVAILABLE", "Service not ready")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	if err := r.check(ctx); err != nil || r.stopping.Load() {
		writeError(w, req, 503, "SERVICE_UNAVAILABLE", "Service not ready")
		return
	}
	writeJSON(w, 200, struct {
		Status string `json:"status"`
	}{Status: "ready"})
}
