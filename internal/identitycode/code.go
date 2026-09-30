package identitycode

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Letters normalizes accents and casing without excluding non-Latin names.
func Letters(value string) string {
	var out strings.Builder
	for _, r := range norm.NFD.String(value) {
		if !unicode.Is(unicode.Mn, r) {
			out.WriteRune(unicode.ToUpper(r))
		}
	}
	return norm.NFC.String(out.String())
}

func Initial(value string) string {
	for _, r := range Letters(value) {
		if unicode.IsLetter(r) {
			return string(r)
		}
	}
	return ""
}

func Prefix(name, lastName string) (string, bool) {
	a, b := Initial(name), Initial(lastName)
	return a + b, a != "" && b != ""
}

func Normalize(value string) (string, bool) {
	value = Letters(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return "", false
	}
	letters := []rune(parts[0])
	if len(letters) != 2 || !unicode.IsLetter(letters[0]) || !unicode.IsLetter(letters[1]) {
		return "", false
	}
	for _, r := range parts[1] {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	n, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || n < 1 {
		return "", false
	}
	return fmt.Sprintf("%s-%d", parts[0], n), true
}

func Effective(id int64, stored string) string {
	if stored != "" {
		return stored
	}
	return fmt.Sprintf("#USER-%04d", id)
}
