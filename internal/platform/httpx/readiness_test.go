package httpx

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessStates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		stop   bool
		status int
	}{{"ready", nil, false, 200}, {"database down", errors.New("private connection string"), false, 503}, {"stopping", nil, true, 503}} {
		t.Run(tt.name, func(t *testing.T) {
			ready := NewReadiness(func(ctx context.Context) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("no deadline")
				}
				return tt.err
			})
			if tt.stop {
				ready.Stop()
			}
			r := NewRouter(quietLogger(), ready)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/ready", nil))
			if w.Code != tt.status {
				t.Fatal(w.Code)
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Fatal("internal error leaked")
			}
			if w.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing request ID")
			}
		})
	}
}
