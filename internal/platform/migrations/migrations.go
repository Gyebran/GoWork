package migrations

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func Open(dsn, directory string) (*migrate.Migrate, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" {
		return nil, errors.New("invalid migration database URL")
	}
	u.Scheme = "pgx5"
	q := u.Query()
	q.Set("connect_timeout", "5")
	q.Set("x-statement-timeout", "30000")
	u.RawQuery = q.Encode()
	path, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	source := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	m, err := migrate.New(source, u.String())
	if err != nil {
		return nil, errors.New("cannot initialize migrations; check connection and migration files")
	}
	m.LockTimeout = 10 * time.Second
	return m, nil
}

func ValidateTestURL(dsn, environment string) error {
	if environment != "test" {
		return errors.New("integration requires APP_ENV=test")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || !strings.HasSuffix(u.Path, "_test") {
		return errors.New("TEST_DATABASE_URL must explicitly target a disposable database ending in _test")
	}
	return nil
}
