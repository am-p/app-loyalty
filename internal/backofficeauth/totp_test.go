package backofficeauth

import (
	"testing"
	"time"
)

func TestValidTOTP(t *testing.T) {
	// RFC 6238 secret and 59-second vector, truncated to six digits.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if !ValidTOTP(secret, "287082", time.Unix(59, 0)) {
		t.Fatal("valid code rejected")
	}
	if ValidTOTP(secret, "287082", time.Unix(359, 0)) {
		t.Fatal("expired code accepted")
	}
	if ValidTOTP(secret, "000000", time.Unix(59, 0)) {
		t.Fatal("incorrect code accepted")
	}
	if ValidTOTP("bad", "287082", time.Unix(59, 0)) {
		t.Fatal("invalid secret accepted")
	}
}
