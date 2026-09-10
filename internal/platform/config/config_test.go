package config

import (
	"log/slog"
	"testing"
)

func TestConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{"defaults", nil, false},
		{"production", map[string]string{"APP_ENV": "production", "PORT": "9000", "LOG_LEVEL": "WARN"}, false},
		{"unknown environment", map[string]string{"APP_ENV": "staging"}, true},
		{"empty environment", map[string]string{"APP_ENV": ""}, true},
		{"zero port", map[string]string{"PORT": "0"}, true},
		{"out of range", map[string]string{"PORT": "65536"}, true},
		{"negative port", map[string]string{"PORT": "-1"}, true},
		{"nonnumeric port", map[string]string{"PORT": "hello"}, true},
		{"empty port", map[string]string{"PORT": ""}, true},
		{"signed port", map[string]string{"PORT": "+80"}, true},
		{"invalid log level", map[string]string{"LOG_LEVEL": "trace"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := load(func(key string) (string, bool) { v, ok := tt.env[key]; return v, ok })
			if (err != nil) != tt.wantErr {
				t.Fatalf("config=%+v err=%v", c, err)
			}
			if tt.name == "defaults" && (c.Port != 8080 || c.Environment != "development" || c.LogLevel != slog.LevelInfo) {
				t.Fatalf("unexpected defaults: %+v", c)
			}
			if tt.name == "production" && (c.Port != 9000 || c.Environment != "production" || c.LogLevel != slog.LevelWarn) {
				t.Fatalf("unexpected config: %+v", c)
			}
		})
	}
}
