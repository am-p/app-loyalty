package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"clientesFrecuentes/internal/model"
)

func TestValidPasswordHonorsBcryptByteLimit(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "minimum", value: "0123456789", valid: true},
		{name: "maximum bytes", value: string(make([]byte, 72)), valid: true},
		{name: "too long for bcrypt", value: string(make([]byte, 73)), valid: false},
		{name: "unicode byte overflow", value: "contraseña-segura-para-pruebas-🔐🔐🔐🔐🔐🔐🔐🔐🔐🔐🔐🔐", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validPassword(tt.value); got != tt.valid {
				t.Fatalf("validPassword byte_length=%d = %v, want %v", len(tt.value), got, tt.valid)
			}
		})
	}
}

func TestAnonymizeRequiresExactConfirmationAndRecentAuthentication(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc := &Service{Now: func() time.Time { return now }}
	if _, err := svc.AnonymizeCurrentUser(context.Background(), 1, now, 1, model.AnonymizeAccountRequest{Confirmation: "BORRAR"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("confirmation error=%v", err)
	}
	if _, err := svc.AnonymizeCurrentUser(context.Background(), 1, now.Add(-10*time.Minute-time.Second), 1, model.AnonymizeAccountRequest{Confirmation: "ANONIMIZAR"}); !errors.Is(err, ErrRecentAuthRequired) {
		t.Fatalf("recent auth error=%v", err)
	}
}
