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
type GoogleIdentity struct {
	GoogleID    string
	Email       string
	Name        string
	LastName    string
	DisplayName string
}

// Legacy verifier kept for existing integrations; signup uses separate claims.
func VerifyGoogleToken(ctx context.Context, token string) (string, string, string, error) {
	identity, err := VerifyGoogleIdentity(ctx, token)
	return identity.GoogleID, identity.Email, identity.DisplayName, err
}

func verifyGoogleToken(ctx context.Context, token string, audiences []string, validate func(context.Context, string, string) (*idtoken.Payload, error)) (string, string, string, error) {
	identity, err := verifyGoogleIdentity(ctx, token, audiences, validate)
	return identity.GoogleID, identity.Email, identity.DisplayName, err
}

func VerifyGoogleIdentity(ctx context.Context, idToken string) (GoogleIdentity, error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	if clientID == "" {
		return GoogleIdentity{}, errors.New("GOOGLE_CLIENT_ID is not set")
	}

	// Android receives an ID token for its own web OAuth client. Keep the web
	// audience valid while accepting that explicitly configured native audience.
	audiences := []string{clientID}
	androidClientID := strings.TrimSpace(os.Getenv("GOOGLE_ANDROID_WEB_CLIENT_ID"))
	if androidClientID != "" && androidClientID != clientID {
		audiences = append(audiences, androidClientID)
	}
	return verifyGoogleIdentity(ctx, idToken, audiences, idtoken.Validate)
}

func verifyGoogleIdentity(ctx context.Context, idToken string, audiences []string, validate func(context.Context, string, string) (*idtoken.Payload, error)) (GoogleIdentity, error) {
	var payload *idtoken.Payload
	var err error
	for _, audience := range audiences {
		payload, err = validate(ctx, idToken, audience)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return GoogleIdentity{}, ErrInvalidGoogleToken
		}
	}
	if err != nil {
		return GoogleIdentity{}, ErrInvalidGoogleToken
	}
	verified, ok := payload.Claims["email_verified"].(bool)
	if !ok || !verified {
		return GoogleIdentity{}, ErrInvalidGoogleToken
	}

	email, ok := payload.Claims["email"].(string)
	if !ok || email == "" {
		return GoogleIdentity{}, ErrInvalidGoogleToken
	}

	name, _ := payload.Claims["name"].(string)
	if name == "" {
		name = email
	}
	givenName, _ := payload.Claims["given_name"].(string)
	familyName, _ := payload.Claims["family_name"].(string)
	return GoogleIdentity{GoogleID: payload.Subject, Email: email, Name: givenName, LastName: familyName, DisplayName: name}, nil
}
