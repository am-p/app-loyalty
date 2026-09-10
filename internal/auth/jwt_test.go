package auth

import (
	"testing"
	"time"
)

func TestTokensRoundTripAndExpiry(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	tokens := NewTokens("01234567890123456789012345678901", "puntazo")
	tokens.Now = func() time.Time { return now }
	raw, err := tokens.Generate(42, "CLIENTE_FINAL")
	if err != nil {
		t.Fatal(err)
	}
	id, kind, err := tokens.Parse(raw)
	if err != nil || id != 42 || kind != "CLIENTE_FINAL" {
		t.Fatalf("round trip: id=%d kind=%s err=%v", id, kind, err)
	}
	tokens.Now = func() time.Time { return now.Add(25 * time.Hour) }
	if _, _, err = tokens.Parse(raw); err == nil {
		t.Fatal("expected expired token")
	}
}

func TestRejectsDifferentSecret(t *testing.T) {
	tokens := NewTokens("01234567890123456789012345678901", "puntazo")
	raw, err := tokens.Generate(1, "PERSONAL_MARCA")
	if err != nil {
		t.Fatal(err)
	}
	other := NewTokens("abcdefghijklmnopqrstuvwxyzABCDEF", "puntazo")
	if _, _, err = other.Parse(raw); err == nil {
		t.Fatal("expected invalid signature")
	}
}

func TestRejectsDifferentIssuer(t *testing.T) {
	issuerA := NewTokens("01234567890123456789012345678901", "puntazo")
	raw, err := issuerA.Generate(1, "CLIENTE_FINAL")
	if err != nil {
		t.Fatal(err)
	}
	issuerB := NewTokens("01234567890123456789012345678901", "another-service")
	if _, _, err = issuerB.Parse(raw); err == nil {
		t.Fatal("expected invalid issuer")
	}
}
