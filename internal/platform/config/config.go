package config

import (
	"fmt"
	"github.com/Gyebran/GoWork/internal/platform/database"
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
	JWTSecret   string
	JWTIssuer   string
	JWTAudience string
	DatabaseURL string
	DBMaxConns  int32
	Environment string
	Port        int
	LogLevel    slog.Level
}

func Load() (Config, error) { return load(os.LookupEnv) }

func load(lookup func(string) (string, bool)) (Config, error) {
	value := func(key, fallback string) string {
		if v, ok := lookup(key); ok {
			return v
		}
		return fallback
	}
	c := Config{Environment: value("APP_ENV", "development")}
	switch c.Environment {
	case "development", "test", "production":
	default:
		return Config{}, fmt.Errorf("APP_ENV must be development, test or production")
	}
	port := value("PORT", "8080")
	for _, r := range port {
		if r < '0' || r > '9' {
			return Config{}, fmt.Errorf("PORT must be an integer from 1 to 65535")
		}
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return Config{}, fmt.Errorf("PORT must be an integer from 1 to 65535")
	}
	c.Port = p
	switch value("LOG_LEVEL", "INFO") {
	case "DEBUG":
		c.LogLevel = slog.LevelDebug
	case "INFO":
		c.LogLevel = slog.LevelInfo
	case "WARN":
		c.LogLevel = slog.LevelWarn
	case "ERROR":
		c.LogLevel = slog.LevelError
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be DEBUG, INFO, WARN or ERROR")
	}
	c.DatabaseURL = value("DATABASE_URL", "")
	max, err := strconv.Atoi(value("DB_MAX_CONNS", "10"))
	if err != nil || max < 1 || max > 50 {
		return Config{}, fmt.Errorf("DB_MAX_CONNS must be between 1 and 50")
	}
	c.DBMaxConns = int32(max)
	if _, err := database.ParseConfig(c.DatabaseURL, c.DBMaxConns, c.Environment == "production"); err != nil {
		return Config{}, err
	}
	c.JWTSecret = value("JWT_SECRET", "")
	c.JWTIssuer = value("JWT_ISSUER", "gowork")
	c.JWTAudience = value("JWT_AUDIENCE", "gowork-api")
	if len(c.JWTSecret) < 32 || c.JWTIssuer == "" || c.JWTAudience == "" {
		return Config{}, fmt.Errorf("JWT_SECRET requires at least 32 bytes; issuer and audience must be nonempty")
	}
	return c, nil
}
