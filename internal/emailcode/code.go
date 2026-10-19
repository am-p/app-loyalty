// Package emailcode owns domain-separated, keyed verification-code fingerprints.
package emailcode

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
)

func Generate() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
func Valid(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func fingerprint(key []byte, domain, value string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("puntazo/email-verification/v1/" + domain + "\x00" + value))
	return mac.Sum(nil)
}
func Hash(key []byte, email, code string) []byte {
	return fingerprint(key, "code", strings.ToLower(strings.TrimSpace(email))+"\x00"+code)
}

// MerchantRegistrationToken is reproducible for an idempotent request without
// storing its bearer secret in the persisted response. The UUID is not the key.
func MerchantRegistrationToken(key []byte, email, idempotencyKey string) string {
	return base64.RawURLEncoding.EncodeToString(fingerprint(key, "registration", email+"\x00"+idempotencyKey))
}
func RegistrationHash(token string) []byte { h := sha256.Sum256([]byte(token)); return h[:] }
