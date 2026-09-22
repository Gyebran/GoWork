package auth

import (
	"context"
	"encoding/json"
	"errors"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const subject = "d5710b16-1d88-4f0b-b752-840a2e486931"
const testSecret = "test-only-signing-secret-with-32-bytes"

func TestTokens(t *testing.T) {
	tokens, _ := NewTokens(testSecret, "gowork", "gowork-api")
	now := time.Unix(1800000000, 0)
	tokens.now = func() time.Time { return now }
	for _, name := range []string{"valid", "expired", "future", "no iat", "no exp", "no sub", "wrong issuer", "wrong audience", "no issuer", "no audience", "non UUID", "too long", "negative ttl", "wrong key", "HS384", "none"} {
		t.Run(name, func(t *testing.T) {
			c := jwt.RegisteredClaims{Subject: subject, Issuer: "gowork", Audience: jwt.ClaimStrings{"gowork-api"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL))}
			method := jwt.SigningMethod(jwt.SigningMethodHS256)
			var key any = []byte(testSecret)
			switch name {
			case "expired":
				c.IssuedAt = jwt.NewNumericDate(now.Add(-time.Hour))
				c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute))
			case "future":
				c.IssuedAt = jwt.NewNumericDate(now.Add(time.Minute))
			case "no iat":
				c.IssuedAt = nil
			case "no exp":
				c.ExpiresAt = nil
			case "no sub":
				c.Subject = ""
			case "wrong issuer":
				c.Issuer = "other"
			case "wrong audience":
				c.Audience = jwt.ClaimStrings{"other"}
			case "no issuer":
				c.Issuer = ""
			case "no audience":
				c.Audience = nil
			case "non UUID":
				c.Subject = "abc"
			case "too long":
				c.ExpiresAt = jwt.NewNumericDate(now.Add(time.Hour))
			case "negative ttl":
				c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second))
			case "wrong key":
				key = []byte("other-test-secret-not-the-same-key")
			case "HS384":
				method = jwt.SigningMethodHS384
			case "none":
				method = jwt.SigningMethodNone
				key = jwt.UnsafeAllowNoneSignatureType
			}
			raw, err := jwt.NewWithClaims(method, c).SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			id, err := tokens.Verify(raw)
			if name == "valid" {
				if err != nil || id != subject {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrUnauthenticated) {
				t.Fatal("invalid token accepted")
			}
		})
	}
	raw, err := tokens.Issue(subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tokens.Verify(raw); err != nil {
		t.Fatal(err)
	}
}

type fakeStore struct {
	login   dbsql.GetLoginUserRow
	current dbsql.GetCurrentUserRow
	err     error
}

func (f *fakeStore) GetLoginUser(context.Context, string) (dbsql.GetLoginUserRow, error) {
	return f.login, f.err
}
func (f *fakeStore) GetCurrentUser(context.Context, pgtype.UUID) (dbsql.GetCurrentUserRow, error) {
	return f.current, f.err
}
func fixture(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	var id pgtype.UUID
	if err = id.Scan(subject); err != nil {
		t.Fatal(err)
	}
	f := &fakeStore{login: dbsql.GetLoginUserRow{ID: id, Email: "test@example.com", Role: "ADMIN", IsActive: true, PasswordHash: string(hash)}, current: dbsql.GetCurrentUserRow{ID: id, Email: "test@example.com", Role: "ADMIN", IsActive: true}}
	tokens, _ := NewTokens(testSecret, "gowork", "gowork-api")
	return &Service{store: f, tokens: tokens, dummyHash: hash}, f
}
func TestLoginAndCurrent(t *testing.T) {
	s, f := fixture(t)
	result, err := s.Login(context.Background(), "test@example.com", "test-password-123")
	if err != nil || result.ExpiresIn != 900 {
		t.Fatal(err)
	}
	u, err := s.Current(context.Background(), result.AccessToken)
	if err != nil || u.ID != subject {
		t.Fatal(err)
	}
	f.current.IsActive = false
	if _, err = s.Current(context.Background(), result.AccessToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled account accepted")
	}
	for _, name := range []string{"wrong password", "missing", "inactive", "database down"} {
		t.Run(name, func(t *testing.T) {
			s, f := fixture(t)
			pw := "test-password-123"
			want := ErrCredentials
			switch name {
			case "wrong password":
				pw = "wrong"
			case "missing":
				f.err = pgx.ErrNoRows
			case "inactive":
				f.login.IsActive = false
			case "database down":
				f.err = errors.New("secret database URI")
				want = ErrUnavailable
			}
			_, err := s.Login(context.Background(), "test@example.com", pw)
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
		})
	}
}
func TestLoginHTTPValidation(t *testing.T) {
	s, _ := fixture(t)
	h := NewHandler(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	router := httpx.NewRouterWithRoutes(slog.Default(), nil, h.Register)
	cases := []struct {
		name, body, media string
		status            int
	}{
		{"valid", `{"email":" TEST@example.com ","password":"test-password-123"}`, "application/json", 200},
		{"duplicate", `{"email":"a@b.c","email":"x@y.z","password":"abc"}`, "application/json", 400},
		{"unknown", `{"email":"a@b.c","password":"abc","role":"ADMIN"}`, "application/json", 422},
		{"null", `{"email":null,"password":"abc"}`, "application/json", 422},
		{"type", `{"email":42}`, "application/json", 400},
		{"missing", `{}`, "application/json", 422},
		{"trailing", `{} {}`, "application/json", 400},
		{"syntax", `{`, "application/json", 400},
		{"media", `{}`, "text/plain", 415},
		{"bytes", `{"email":"a@b.c","password":"` + strings.Repeat("é", 37) + `"}`, "application/json", 422},
		{"oversize", strings.Repeat(" ", httpx.MaxBodyBytes+1), "application/json", 413},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(c.body))
			r.Header.Set("Content-Type", c.media)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != c.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cache header missing")
			}
			if strings.Contains(w.Body.String(), "password_hash") {
				t.Fatal("hash leaked")
			}
		})
	}
	for _, header := range []string{"", "Basic abc", "Bearer invalid"} {
		r := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Fatal("invalid bearer accepted")
		}
	}
}
func TestPasswordBounds(t *testing.T) {
	for _, pw := range []string{"short", strings.Repeat("é", 37)} {
		if _, err := HashPassword(pw); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	pw := strings.Repeat("é", 6)
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) != nil {
		t.Fatal("bcrypt comparison failed")
	}
	cost, _ := bcrypt.Cost([]byte(hash))
	if cost != PasswordCost {
		t.Fatal(cost)
	}
	body, _ := json.Marshal(User{ID: subject})
	if strings.Contains(string(body), "password") {
		t.Fatal("sensitive model field")
	}
}
func BenchmarkPasswordHash(b *testing.B) {
	for b.Loop() {
		if _, err := HashPassword("benchmark-only-password"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestBootstrapRejectsNULBeforeDatabase(t *testing.T) {
	if _, err := Bootstrap(context.Background(), nil, "name\x00suffix", "admin@example.com", "long-password-2026"); err == nil {
		t.Fatal("NUL accepted")
	}
}
