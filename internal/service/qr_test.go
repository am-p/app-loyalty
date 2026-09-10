package service

import (
	"bytes"
	"testing"

	"clientesFrecuentes/internal/config"
)

func TestQRIsStableOpaqueAndPeppered(t *testing.T) {
	s := &Service{Config: config.Config{QRPepper: "pepper-01234567890123456789012345"}}
	token, hash := s.QRForUser(9)
	if len(token) < 32 {
		t.Fatalf("token too short: %d", len(token))
	}
	again, againHash := s.QRForUser(9)
	if token != again || !bytes.Equal(hash, againHash) {
		t.Fatal("QR must be stable")
	}
	other, _ := s.QRForUser(10)
	if token == other {
		t.Fatal("QR tokens must differ")
	}
	if bytes.Contains(hash, []byte(token)) {
		t.Fatal("stored hash leaked token")
	}
	if !bytes.Equal(hash, s.QRHash(token)) {
		t.Fatal("lookup hash mismatch")
	}
}
