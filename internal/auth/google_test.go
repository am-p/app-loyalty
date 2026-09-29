package auth

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"google.golang.org/api/idtoken"
)

func TestVerifyGoogleTokenAcceptsConfiguredAndroidAudience(t *testing.T) {
	var tried []string
	validate := func(_ context.Context, _ string, audience string) (*idtoken.Payload, error) {
		tried = append(tried, audience)
		if audience != "android-web-client" {
			return nil, errors.New("audience mismatch")
		}
		return &idtoken.Payload{Subject: "google-user", Claims: map[string]interface{}{
			"email_verified": true,
			"email":          "person@example.com",
			"name":           "Person",
		}}, nil
	}

	id, email, name, err := verifyGoogleToken(context.Background(), "token", []string{"web-client", "android-web-client"}, validate)
	if err != nil || id != "google-user" || email != "person@example.com" || name != "Person" {
		t.Fatalf("Google token rejected: id=%q email=%q name=%q err=%v", id, email, name, err)
	}
	if !reflect.DeepEqual(tried, []string{"web-client", "android-web-client"}) {
		t.Fatalf("unexpected audiences: %v", tried)
	}
}

func TestVerifyGoogleTokenRejectsUnknownAudience(t *testing.T) {
	var tried []string
	validate := func(_ context.Context, _ string, audience string) (*idtoken.Payload, error) {
		tried = append(tried, audience)
		return nil, errors.New("audience mismatch")
	}

	_, _, _, err := verifyGoogleToken(context.Background(), "foreign-token", []string{"web-client", "android-web-client"}, validate)
	if !errors.Is(err, ErrInvalidGoogleToken) {
		t.Fatalf("expected invalid Google token, got %v", err)
	}
	if !reflect.DeepEqual(tried, []string{"web-client", "android-web-client"}) {
		t.Fatalf("unexpected audiences: %v", tried)
	}
}

func TestVerifyGoogleTokenRejectsUnverifiedEmail(t *testing.T) {
	validate := func(_ context.Context, _ string, _ string) (*idtoken.Payload, error) {
		return &idtoken.Payload{Subject: "google-user", Claims: map[string]interface{}{
			"email_verified": false,
			"email":          "person@example.com",
		}}, nil
	}
	_, _, _, err := verifyGoogleToken(context.Background(), "token", []string{"web-client"}, validate)
	if !errors.Is(err, ErrInvalidGoogleToken) {
		t.Fatalf("expected invalid Google token, got %v", err)
	}
}
