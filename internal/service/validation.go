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

// bcrypt rejects passwords longer than 72 bytes. Validate the encoded input
// before hashing so a contract error cannot become a 500 response.
func validPassword(value string) bool { return len(value) >= 10 && len(value) <= 72 }
