package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateDatabaseTargetAllowsOnlyDedicatedLoopbackDatabase(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		{"ipv4 dedicated fixture database", "postgres://fixture:secret@127.0.0.1:55437/puntazo_load?sslmode=disable", true},
		{"localhost dedicated fixture database", "postgres://fixture:secret@localhost:55437/puntazo_load?sslmode=disable", true},
		{"ipv6 dedicated fixture database", "postgres://fixture:secret@[::1]:55437/puntazo_load?sslmode=disable", true},
		{"remote host", "postgres://fixture:secret@db.example.test:55437/puntazo_load?sslmode=disable", false},
		{"wrong port", "postgres://fixture:secret@127.0.0.1:5432/puntazo_load?sslmode=disable", false},
		{"wrong database", "postgres://fixture:secret@127.0.0.1:55437/postgres?sslmode=disable", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateDatabaseTarget(tt.dsn)
			if (err == nil) != tt.want {
				t.Fatalf("validateDatabaseTarget() error = %v, want allowed=%t", err, tt.want)
			}
		})
	}
}

func TestRandomPasswordIsSufficientForSyntheticFixtureHash(t *testing.T) {
	password, err := randomPassword()
	if err != nil {
		t.Fatalf("randomPassword() error = %v", err)
	}
	if len(password) < fixturePasswordLen {
		t.Fatalf("random password length = %d, want at least %d", len(password), fixturePasswordLen)
	}
}

func TestCredentialOutputMustBeOutsideRepository(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate fixture helper")
	}
	repo := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../"))
	if !pathWithinRepository(filepath.Join(repo, "scripts", "stress-remediation", "credentials.json")) {
		t.Fatal("credential output under repository was not rejected")
	}
	external := filepath.Join(t.TempDir(), "credentials.json")
	if pathWithinRepository(external) {
		if _, err := os.Stat(repo); err != nil {
			t.Fatalf("repository path unavailable for containment check: %v", err)
		}
		t.Fatal("temporary external credential output was rejected")
	}
}
