package config

import "testing"

func TestFourDigits(t *testing.T) {
	for _, value := range []string{"0001", "9999"} {
		if !fourDigits(value) {
			t.Fatalf("valid schema version rejected: %s", value)
		}
	}
	for _, value := range []string{"001", "00001", "00a1", "abcd"} {
		if fourDigits(value) {
			t.Fatalf("invalid schema version accepted: %s", value)
		}
	}
}

func TestJWTIssuerDefaultsToPuntazo(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("JWT_ISSUER", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWTIssuer != "puntazo" {
		t.Fatalf("JWTIssuer = %q, want puntazo", cfg.JWTIssuer)
	}
}

func TestJWTIssuerCanBeConfigured(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("JWT_ISSUER", "puntazo-staging")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWTIssuer != "puntazo-staging" {
		t.Fatalf("JWTIssuer = %q, want puntazo-staging", cfg.JWTIssuer)
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://puntazo:puntazo@localhost:5432/puntazo")
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("QR_PEPPER", "abcdefghijklmnopqrstuvwxyzABCDEF")
}
