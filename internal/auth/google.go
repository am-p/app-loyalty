package auth

import (
	"context"
	"errors"
	"os"

	"google.golang.org/api/idtoken"
)

var ErrInvalidGoogleToken = errors.New("invalid google token")

func VerifyGoogleToken(idToken string) (googleID, email, name string, err error) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		return "", "", "", errors.New("GOOGLE_CLIENT_ID is not set")
	}

	payload, err := idtoken.Validate(context.Background(), idToken, clientID)
	if err != nil {
		return "", "", "", ErrInvalidGoogleToken
	}
	verified, ok := payload.Claims["email_verified"].(bool)
	if !ok || !verified {
		return "", "", "", ErrInvalidGoogleToken
	}

	email, ok = payload.Claims["email"].(string)
	if !ok || email == "" {
		return "", "", "", ErrInvalidGoogleToken
	}

	name, _ = payload.Claims["name"].(string)
	if name == "" {
		name = email
	}
	return payload.Subject, email, name, nil
}
