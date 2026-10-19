package emailcode

import (
	"bytes"
	"testing"
)

func TestCodeFormatAndKeyedScope(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	for range 100 {
		code, err := Generate()
		if err != nil || !Valid(code) {
			t.Fatalf("invalid generated code err=%v", err)
		}
	}
	if !Valid("000001") || Valid("12345") || Valid("１２３４５６") || Valid("12a456") {
		t.Fatal("code format validation")
	}
	hash := Hash(key, "a@example.test", "000001")
	if bytes.Equal(hash, Hash(key, "b@example.test", "000001")) || bytes.Equal(hash, Hash([]byte("other"), "a@example.test", "000001")) {
		t.Fatal("missing keyed account scope")
	}
	if !bytes.Equal(hash, Hash(key, " A@example.test ", "000001")) {
		t.Fatal("normalization")
	}
	first := MerchantRegistrationToken(key, "a@example.test", "request-one")
	if first != MerchantRegistrationToken(key, "a@example.test", "request-one") || first == MerchantRegistrationToken(key, "a@example.test", "request-two") {
		t.Fatal("idempotent registration proof")
	}
}
