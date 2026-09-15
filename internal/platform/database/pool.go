package database

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const SchemaVersion = 9

// ParseConfig never returns the connection string in errors.
func ParseConfig(dsn string, max int32, production bool) (*pgxpool.Config, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" {
		return nil, errors.New("DATABASE_URL must be a PostgreSQL URL with host and database")
	}
	if max < 1 || max > 50 {
		return nil, errors.New("DB_MAX_CONNS must be between 1 and 50")
	}
	if production && u.Query().Get("sslmode") != "verify-full" {
		return nil, errors.New("production DATABASE_URL requires sslmode=verify-full")
	}
	c, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	c.MaxConns = max
	c.MinConns = 0
	c.ConnConfig.ConnectTimeout = 5 * time.Second
	c.MaxConnLifetime = 30 * time.Minute
	c.MaxConnIdleTime = 5 * time.Minute
	return c, nil
}

// Pool construction is lazy: /health stays live during a DB outage; /ready fails.
func Open(ctx context.Context, dsn string, max int32, production bool) (*pgxpool.Pool, error) {
	c, err := ParseConfig(dsn, max, production)
	if err != nil {
		return nil, err
	}
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, errors.New("database pool initialization failed")
	}
	return p, nil
}

func Ready(ctx context.Context, p *pgxpool.Pool) error {
	var version int
	var dirty bool
	if err := p.QueryRow(ctx, "SELECT version, dirty FROM public.schema_migrations").Scan(&version, &dirty); err != nil {
		return errors.New("database or schema unavailable")
	}
	if version != SchemaVersion || dirty {
		return errors.New("database migrations not current")
	}
	return nil
}
