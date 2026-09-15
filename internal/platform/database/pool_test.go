package database

import (
	"strings"
	"testing"
)

func TestPoolConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name, dsn  string
		max        int32
		prod, fail bool
	}{
		{"local", "postgres://localhost/gowork?sslmode=disable", 10, false, false},
		{"production", "postgres://localhost/gowork?sslmode=verify-full", 10, true, false},
		{"no TLS", "postgres://localhost/gowork?sslmode=disable", 10, true, true},
		{"optional TLS", "postgres://localhost/gowork?sslmode=prefer", 10, true, true},
		{"empty", "", 10, false, true}, {"invalid", "secret-password", 10, false, true},
		{"zero max", "postgres://localhost/gowork", 0, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseConfig(tt.dsn, tt.max, tt.prod)
			if (err != nil) != tt.fail {
				t.Fatalf("unexpected error %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-password") {
				t.Fatal("leaked input")
			}
			if !tt.fail && c.MaxConns != tt.max {
				t.Fatal("pool bound ignored")
			}
		})
	}
}
