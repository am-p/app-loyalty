package service

import (
	"strings"
	"testing"
)

func TestNormalizeStoreSearch(t *testing.T) {
	for _, tt := range []struct {
		in, want string
		bad      bool
	}{
		{"  café  ", "café", false}, {"", "", false}, {strings.Repeat("ñ", 200), strings.Repeat("ñ", 200), false},
		{strings.Repeat("ñ", 201), "", true}, {"test\x00", "", true}, {string([]byte{0xff}), "", true},
	} {
		got, err := NormalizeStoreSearch(tt.in)
		if (err != nil) != tt.bad || got != tt.want {
			t.Fatalf("got=%q err=%v", got, err)
		}
	}
}
