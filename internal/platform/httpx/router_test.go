package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }
func TestHealthAndRequestIDs(t *testing.T) {
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, id := range []string{"", "client_123.test-1", "bad id", strings.Repeat("a", 65)} {
		t.Run(id, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/health", nil)
			if id != "" {
				r.Header.Set("X-Request-ID", id)
			}
			w := httptest.NewRecorder()
			NewRouter(quietLogger()).ServeHTTP(w, r)
			if w.Code != 200 || w.Body.String() != "{\"status\":\"ok\"}\n" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response: %d %s %v", w.Code, w.Body.String(), w.Header())
			}
			got := w.Header().Get("X-Request-ID")
			if id == "client_123.test-1" {
				if got != id {
					t.Fatal(got)
				}
			} else if !uuid.MatchString(got) {
				t.Fatalf("not a UUID: %q", got)
			}
		})
	}
}
func TestDuplicateRequestIDIsReplaced(t *testing.T) {
	r := httptest.NewRequest("GET", "/health", nil)
	r.Header.Add("X-Request-ID", "first")
	r.Header.Add("X-Request-ID", "second")
	w := httptest.NewRecorder()
	NewRouter(quietLogger()).ServeHTTP(w, r)
	if id := w.Header().Get("X-Request-ID"); id == "first" || id == "second" || id == "" {
		t.Fatal(id)
	}
}
func TestRoutingErrors(t *testing.T) {
	for _, tt := range []struct {
		method, path string
		status       int
		code         string
	}{{"GET", "/missing", 404, "NOT_FOUND"}, {"GET", "/ready", 404, "NOT_FOUND"}, {"POST", "/health", 405, "METHOD_NOT_ALLOWED"}} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			NewRouter(quietLogger()).ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			var b errorBody
			if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
				t.Fatal(err)
			}
			if w.Code != tt.status || b.Error.Code != tt.code || b.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatalf("%d %+v", w.Code, b)
			}
			if tt.status == 405 && w.Header().Get("Allow") != "GET" {
				t.Fatal("missing Allow")
			}
		})
	}
}
func TestAccessLogRedactsRequestData(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	r := httptest.NewRequest("GET", "/secret-path?password=secret-query", strings.NewReader("secret-body"))
	r.Header.Set("Authorization", "Bearer secret-token")
	r.Header.Set("X-Request-ID", "trace-1")
	w := httptest.NewRecorder()
	NewRouter(logger).ServeHTTP(w, r)
	if strings.Contains(output.String(), "secret-") {
		t.Fatal("sensitive request data logged")
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["request_id"] != "trace-1" || entry["status"] != float64(404) || entry["route"] != "[unmatched]" {
		t.Fatal(entry)
	}
}
func TestTimeoutCancelsContextAndReturnsJSON(t *testing.T) {
	done := make(chan struct{})
	h := requestIDs(requestTimeout(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(done)
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/slow", nil))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("context not cancelled")
	}
	var b errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if w.Code != 503 || w.Header().Get("Content-Type") != "application/json" || b.Error.Code != "SERVICE_UNAVAILABLE" || b.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatalf("%d %v %+v", w.Code, w.Header(), b)
	}
}
func TestParentCancellationReachesHandler(t *testing.T) {
	started, done := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := requestIDs(requestTimeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(done) })))
	result := make(chan struct{})
	go func() {
		defer close(result)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/slow", nil).WithContext(ctx))
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation not propagated")
	}
	<-result
}
func TestPanicIsSanitizedAndBufferedOutputDiscarded(t *testing.T) {
	var log bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&log, nil))
	h := requestIDs(recoverPanic(logger)(requestTimeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial-secret"))
		panic("secret-panic")
	}))))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/panic", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") || strings.Contains(log.String(), "secret") {
		t.Fatalf("response=%s log=%s", w.Body.String(), log.String())
	}
	var b errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Error.Code != "INTERNAL_ERROR" || b.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatal(b)
	}
}
