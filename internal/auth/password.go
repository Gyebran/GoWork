package auth

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const PasswordCost = 12

func NormalizeEmail(input string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(input))
	if len(email) < 3 || len(email) > 254 {
		return "", errors.New("invalid email")
	}
	for _, r := range email {
		if r > 127 {
			return "", errors.New("email must be ASCII")
		}
	}
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || a.Name != "" {
		return "", errors.New("invalid email")
	}
	return email, nil
}
func HashPassword(password string) (string, error) {
	if !utf8.ValidString(password) || len(password) < 12 || len(password) > 72 {
		return "", errors.New("password must contain 12 to 72 UTF-8 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	return string(hash), err
}
