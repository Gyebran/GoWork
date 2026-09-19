package auth

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"regexp"
	"time"
)

const TokenTTL = 15 * time.Minute

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var ErrUnauthenticated = errors.New("unauthenticated")

type Tokens struct {
	secret           []byte
	issuer, audience string
	now              func() time.Time
}

func NewTokens(secret, issuer, audience string) (*Tokens, error) {
	if len(secret) < 32 || issuer == "" || audience == "" {
		return nil, errors.New("JWT secret must be at least 32 bytes and issuer/audience required")
	}
	return &Tokens{secret: []byte(secret), issuer: issuer, audience: audience, now: time.Now}, nil
}
func (t *Tokens) Issue(subject string) (string, error) {
	if !uuidPattern.MatchString(subject) {
		return "", ErrUnauthenticated
	}
	now := t.now().UTC()
	c := jwt.RegisteredClaims{Subject: subject, Issuer: t.issuer, Audience: jwt.ClaimStrings{t.audience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL))}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
}
func (t *Tokens) Verify(raw string) (string, error) {
	if len(raw) > 8192 {
		return "", ErrUnauthenticated
	}
	c := new(jwt.RegisteredClaims)
	token, err := jwt.ParseWithClaims(raw, c, func(token *jwt.Token) (any, error) { return t.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithIssuer(t.issuer), jwt.WithAudience(t.audience), jwt.WithLeeway(30*time.Second), jwt.WithTimeFunc(t.now))
	if err != nil || !token.Valid || c.IssuedAt == nil || c.ExpiresAt == nil || !uuidPattern.MatchString(c.Subject) {
		return "", ErrUnauthenticated
	}
	ttl := c.ExpiresAt.Sub(c.IssuedAt.Time)
	if ttl <= 0 || ttl > TokenTTL {
		return "", ErrUnauthenticated
	}
	return c.Subject, nil
}
