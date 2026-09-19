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
	if env != "development" && env != "test" {
		log.Error("demo seed requires development or test")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"), 1, false)
	if err != nil {
		log.Error("invalid database configuration")
		return 1
	}
	defer pool.Close()
	if database.Ready(ctx, pool) != nil {
		log.Error("database not ready")
		return 1
	}
	if auth.SeedDemo(ctx, pool, env) != nil {
		log.Error("demo seed failed; existing users are never overwritten")
		return 1
	}
	log.Info("demo_seed_complete")
	return 0
}
