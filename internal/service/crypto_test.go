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

func TestFingerprintStableAndSensitive(t *testing.T) {
	a := Fingerprint(struct {
		A string `json:"a"`
	}{"one"})
	b := Fingerprint(struct {
		A string `json:"a"`
	}{"one"})
	c := Fingerprint(struct {
		A string `json:"a"`
	}{"two"})
	if !bytes.Equal(a, b) {
		t.Fatal("same request changed fingerprint")
	}
	if bytes.Equal(a, c) {
		t.Fatal("different request kept fingerprint")
	}
}

func TestKeyedFingerprintDoesNotPersistPlainSHAOracle(t *testing.T) {
	request := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{"owner@example.com", "guessable-password"}
	plain := Fingerprint(request)
	keyed := KeyedFingerprint("server-secret-that-is-not-in-the-database", request)
	again := KeyedFingerprint("server-secret-that-is-not-in-the-database", request)
	other := KeyedFingerprint("different-server-secret", request)
	if bytes.Equal(plain, keyed) {
		t.Fatal("sensitive fingerprint must not equal the unkeyed request hash")
	}
	if !bytes.Equal(keyed, again) {
		t.Fatal("keyed fingerprint must be stable for idempotency")
	}
	if bytes.Equal(keyed, other) {
		t.Fatal("keyed fingerprint must depend on its server-side key")
	}
}
