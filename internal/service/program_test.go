package service

import "testing"

func TestNormalizeProgramType(t *testing.T) {
	for input, want := range map[string]string{"sellos": "SELLOS", " PUNTOS ": "PUNTOS"} {
		got, err := normalizeProgramType(input)
		if err != nil || got != want {
			t.Fatalf("normalizeProgramType(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "AMBOS", "PUNTAJES"} {
		if _, err := normalizeProgramType(input); err == nil {
			t.Fatalf("normalizeProgramType(%q) unexpectedly succeeded", input)
		}
	}
}
