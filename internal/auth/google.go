package auth

import (
	"context"
	"errors"
	"os"
	"strings"

	"google.golang.org/api/idtoken"
)

var ErrInvalidGoogleToken = errors.New("invalid google token")

// VerifyGoogleToken propagates request cancellation to Google's public-key
// download when the validator's certificate cache needs refreshing.
func VerifyGoogleToken(ctx context.Context, idToken string) (googleID, email, name string, err error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	if clientID == "" {
		return "", "", "", errors.New("GOOGLE_CLIENT_ID is not set")
	}

	// Android receives an ID token for its own web OAuth client. Keep the web
	// audience valid while accepting that explicitly configured native audience.
	audiences := []string{clientID}
	androidClientID := strings.TrimSpace(os.Getenv("GOOGLE_ANDROID_WEB_CLIENT_ID"))
	if androidClientID != "" && androidClientID != clientID {
		audiences = append(audiences, androidClientID)
	}
	return verifyGoogleToken(ctx, idToken, audiences, idtoken.Validate)
}

func verifyGoogleToken(ctx context.Context, idToken string, audiences []string, validate func(context.Context, string, string) (*idtoken.Payload, error)) (googleID, email, name string, err error) {
	var payload *idtoken.Payload
	for _, audience := range audiences {
		payload, err = validate(ctx, idToken, audience)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return "", "", "", ErrInvalidGoogleToken
		}
	}
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
