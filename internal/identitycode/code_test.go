package identitycode

import "testing"

func TestPrefixes(t *testing.T) {
	for _, tc := range []struct {
		name, last, want string
		valid            bool
	}{
		{"Gabriel", "Gonzalez", "GG", true}, {"Gabriel", "Gutierrez", "GG", true}, {"Gonzalo", "Tevez", "GT", true},
		{"  María José  ", " Pérez López ", "MP", true}, {"Ángel", "Ñúñez", "AN", true}, {"E\u0301rica", "O\u0301rtiz", "EO", true},
		{"", "Gonzalez", "G", false}, {"123", "---", "", false}, {" Gabriel ", "", "G", false},
	} {
		got, ok := Prefix(tc.name, tc.last)
		if got != tc.want || ok != tc.valid {
			t.Errorf("%q/%q: %q %v", tc.name, tc.last, got, ok)
		}
	}
}
func TestNormalize(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{" #gg-01 ", "GG-1"}, {"mp-123", "MP-123"}, {"áñ-1", "AN-1"}, {"GG-0", ""}, {"GG--1", ""}, {"GG-+1", ""}, {"GG-9223372036854775808", ""}, {"USER-0001", ""}, {"GG-1x", ""}} {
		got, ok := Normalize(tc.input)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%q => %q %v", tc.input, got, ok)
		}
	}
	if Effective(1, "") != "#USER-0001" || Effective(12345, "") != "#USER-12345" || Effective(1, "GG-1") != "GG-1" {
		t.Fatal("effective codes")
	}
}
