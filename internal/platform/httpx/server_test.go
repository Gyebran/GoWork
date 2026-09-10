package httpx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

type observedListener struct {
	net.Listener
	closed chan struct{}
}

func (l *observedListener) Close() error {
	err := l.Listener.Close()
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return err
}
func listener(t *testing.T) *observedListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &observedListener{Listener: ln, closed: make(chan struct{})}
	t.Cleanup(func() { _ = l.Close() })
	return l
}
func TestGracefulShutdownDrainsInFlightRequest(t *testing.T) {
	ln := listener(t)
	started, release := make(chan struct{}), make(chan struct{})
	srv := NewServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	}), quietLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- serve(ctx, srv, ln, quietLogger(), time.Second) }()
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		res, err := client.Get("http://" + ln.Addr().String())
		if err != nil {
			response <- err
			return
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err == nil && string(b) != "finished" {
			err = errors.New("response was not drained")
		}
		response <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case <-ln.closed:
	case <-time.After(time.Second):
		t.Fatal("listener not closed")
	}
	select {
	case err := <-result:
		t.Fatalf("shutdown returned before request completed: %v", err)
	default:
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}
func TestShutdownDeadlineForcesConnectionClose(t *testing.T) {
	ln := listener(t)
	started, done := make(chan struct{}), make(chan struct{})
	srv := NewServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(done) }), quietLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- serve(ctx, srv, ln, quietLogger(), 20*time.Millisecond) }()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		client := &http.Client{Timeout: time.Second}
		res, err := client.Get("http://" + ln.Addr().String())
		if err == nil {
			res.Body.Close()
		}
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown deadline: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("active connection was not closed")
	}
	<-clientDone
}
func TestServeFailurePropagates(t *testing.T) {
	ln := listener(t)
	_ = ln.Close()
	if err := serve(context.Background(), NewServer("", NewRouter(quietLogger()), quietLogger()), ln, quietLogger(), time.Second); err == nil {
		t.Fatal("closed listener error swallowed")
	}
}
