package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Gyebran/GoWork/internal/platform/config"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
)

func main() { os.Exit(run()) }

func run() int {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("invalid_configuration", "error", err.Error())
		return 1
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := httpx.NewServer(net.JoinHostPort("0.0.0.0", strconv.Itoa(cfg.Port)), httpx.NewRouter(logger), logger)
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		logger.Error("listen_failed", "error", err.Error())
		return 1
	}
	if err := httpx.Serve(ctx, server, ln, logger); err != nil {
		logger.Error("server_failed", "error", err.Error())
		return 1
	}
	return 0
}
