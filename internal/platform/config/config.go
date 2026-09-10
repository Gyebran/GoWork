package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
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
	return c, nil
}
