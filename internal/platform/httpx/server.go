package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const ShutdownGrace = 20 * time.Second

func NewServer(address string, handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second,
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError)}
}

// Serve owns listener/server lifetime. Request contexts intentionally do not use
// the signal context: shutdown should drain requests rather than cancel them.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, logger *slog.Logger) error {
	return serve(ctx, srv, ln, logger, ShutdownGrace)
}
func serve(ctx context.Context, srv *http.Server, ln net.Listener, logger *slog.Logger, grace time.Duration) error {
	result := make(chan error, 1)
	go func() { result <- srv.Serve(ln) }()
	logger.Info("server_started", "address", ln.Addr().String())
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		logger.Info("server_stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			closeErr := srv.Close()
			<-result
			return errors.Join(fmt.Errorf("graceful shutdown: %w", err), closeErr)
		}
		if err := <-result; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		logger.Info("server_stopped")
		return nil
	}
}
