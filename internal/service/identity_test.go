package service

import (
	"testing"

	"clientesFrecuentes/internal/config"
)

func TestResolveMovementIdentityAcceptsQRAndMemberCode(t *testing.T) {
	s := &Service{Config: config.Config{QRPepper: "pepper-01234567890123456789012345"}}
	wantToken, _ := s.QRForUser(9)

	for _, code := range []string{"#USER-0009", "USER-0009", "#user-0009"} {
		got, err := s.resolveMovementIdentity("", code)
		if err != nil || got != wantToken {
			t.Fatalf("code %q resolved token=%q err=%v", code, got, err)
		}
	}
	got, err := s.resolveMovementIdentity(wantToken, "")
	if err != nil || got != wantToken {
		t.Fatalf("QR token changed: token=%q err=%v", got, err)
	}
}

func TestResolveMovementIdentityRejectsAmbiguousOrInvalidValues(t *testing.T) {
	s := &Service{Config: config.Config{QRPepper: "pepper-01234567890123456789012345"}}
	token, _ := s.QRForUser(9)

	tests := []struct{ qr, code string }{
		{},
		{qr: token, code: "#USER-0009"},
		{qr: "short"},
		{code: "9"},
		{code: "#USER-0000"},
		{code: "#USER-ABC9"},
	}
	for _, test := range tests {
		if _, err := s.resolveMovementIdentity(test.qr, test.code); err == nil {
			t.Fatalf("expected rejection for qr=%q code=%q", test.qr, test.code)
		}
	}
}
