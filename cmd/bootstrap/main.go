package main

import (
	"context"
	"github.com/Gyebran/GoWork/internal/auth"
	"github.com/Gyebran/GoWork/internal/platform/database"
	"log/slog"
	"os"
	"time"
)

func main() { os.Exit(run()) }
func run() int {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	env := os.Getenv("APP_ENV")
	if env != "development" && env != "test" && env != "production" {
		log.Error("APP_ENV must be explicitly set")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"), 1, env == "production")
	if err != nil {
		log.Error("invalid database configuration")
		return 1
	}
	defer pool.Close()
	if database.Ready(ctx, pool) != nil {
		log.Error("database not ready")
		return 1
	}
	_, err = auth.Bootstrap(ctx, pool, os.Getenv("BOOTSTRAP_ADMIN_NAME"), os.Getenv("BOOTSTRAP_ADMIN_EMAIL"), os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"))
	if err != nil {
		log.Error("bootstrap failed; check input, existing administrator, and database permissions")
		return 1
	}
	log.Info("administrator_bootstrapped")
	return 0
}
