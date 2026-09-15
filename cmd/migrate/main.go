package main

import (
	"errors"
	"github.com/Gyebran/GoWork/internal/platform/database"
	"github.com/Gyebran/GoWork/internal/platform/migrations"
	"github.com/golang-migrate/migrate/v4"
	"log/slog"
	"os"
)

func main() { os.Exit(run()) }
func run() int {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down") {
		log.Error("usage: go run ./cmd/migrate up|down")
		return 1
	}
	env := os.Getenv("APP_ENV")
	if os.Args[1] == "down" && env != "development" && env != "test" {
		log.Error("down migrations require explicit development or test environment")
		return 1
	}
	dsn := os.Getenv("MIGRATION_DATABASE_URL")
	if _, err := database.ParseConfig(dsn, 1, env == "production"); err != nil {
		log.Error("invalid migration database configuration")
		return 1
	}
	m, err := migrations.Open(dsn, "db/migrations")
	if err != nil {
		log.Error("migration initialization failed")
		return 1
	}
	if os.Args[1] == "up" {
		err = m.Up()
	} else {
		err = m.Steps(-1)
	}
	a, b := m.Close()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Error("migration failed; inspect database migration version and server logs")
		return 1
	}
	if a != nil || b != nil {
		log.Error("migration cleanup failed")
		return 1
	}
	log.Info("migration_complete", "direction", os.Args[1])
	return 0
}
