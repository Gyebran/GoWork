package auth

import (
	"context"
	"errors"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
	"time"
)

var ErrCredentials = errors.New("invalid credentials")
var ErrUnavailable = errors.New("authentication storage unavailable")

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type LoginResult struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	User        User   `json:"user"`
}
type Store interface {
	GetLoginUser(context.Context, string) (dbsql.GetLoginUserRow, error)
	GetCurrentUser(context.Context, pgtype.UUID) (dbsql.GetCurrentUserRow, error)
}
type Service struct {
	store     Store
	tokens    *Tokens
	dummyHash []byte
}

func NewService(store Store, tokens *Tokens) (*Service, error) {
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy-comparison-not-an-account"), PasswordCost)
	if err != nil {
		return nil, err
	}
	return &Service{store: store, tokens: tokens, dummyHash: dummy}, nil
}
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	u, err := s.store.GetLoginUser(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrUnavailable
	}
	hash := []byte(u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		hash = s.dummyHash
	}
	compareErr := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if err != nil || compareErr != nil || !u.IsActive {
		return LoginResult{}, ErrCredentials
	}
	if ctx.Err() != nil {
		return LoginResult{}, ErrUnavailable
	}
	token, err := s.tokens.Issue(u.ID.String())
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: token, TokenType: "Bearer", ExpiresIn: 900, User: publicUser(u.ID, u.Name, u.Email, u.Role, u.IsActive, u.CreatedAt, u.UpdatedAt)}, nil
}
func (s *Service) Current(ctx context.Context, raw string) (User, error) {
	subject, err := s.tokens.Verify(raw)
	if err != nil {
		return User{}, ErrUnauthenticated
	}
	var id pgtype.UUID
	if err = id.Scan(subject); err != nil {
		return User{}, ErrUnauthenticated
	}
	u, err := s.store.GetCurrentUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, ErrUnavailable
	}
	if !u.IsActive {
		return User{}, ErrUnauthenticated
	}
	return publicUser(u.ID, u.Name, u.Email, u.Role, u.IsActive, u.CreatedAt, u.UpdatedAt), nil
}
func publicUser(id pgtype.UUID, name, email, role string, active bool, created, updated pgtype.Timestamptz) User {
	return User{ID: id.String(), Name: name, Email: email, Role: role, IsActive: active, CreatedAt: created.Time.UTC(), UpdatedAt: updated.Time.UTC()}
}
