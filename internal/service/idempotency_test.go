package service

import (
	"bytes"
	"testing"
)

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
