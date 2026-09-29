package backofficeauth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// ValidTOTP accepts a small clock drift and consumes a six-digit code.
func ValidTOTP(secret, code string, now time.Time) bool {
	_, ok := MatchingTOTPStep(secret, code, now)
	return ok
}

func MatchingTOTPStep(secret, code string, now time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) < 16 {
		return 0, false
	}
	for delta := -1; delta <= 1; delta++ {
		step := now.Unix()/30 + int64(delta)
		if step < 0 {
			continue
		}
		counter := uint64(step)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], counter)
		mac := hmac.New(sha1.New, key)
		_, _ = mac.Write(b[:])
		sum := mac.Sum(nil)
		offset := sum[len(sum)-1] & 15
		value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
		candidate := fmt.Sprintf("%06d", value%1000000)
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
