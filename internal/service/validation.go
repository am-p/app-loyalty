package service

import (
	"net/mail"
	"strings"
)

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 254 {
		return "", ErrInvalidRequest
	}
	return value, nil
}

func cleanName(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > max {
		return "", ErrInvalidRequest
	}
	return value, nil
}

func validPassword(value string) bool { return len(value) >= 10 && len(value) <= 128 }
