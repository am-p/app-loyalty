package config

import "testing"

func TestFourDigits(t *testing.T) {
	for _, value := range []string{"0001", "9999"} {
		if !fourDigits(value) {
			t.Fatalf("valid schema version rejected: %s", value)
		}
	}
	for _, value := range []string{"001", "00001", "00a1", "abcd"} {
		if fourDigits(value) {
			t.Fatalf("invalid schema version accepted: %s", value)
		}
	}
}
