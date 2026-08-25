package stringx

import (
	"strings"
	"unicode"
)

func ToSnakeCase(s string) string {
	if s == "" {
		return ""
	}

	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 2)

	for i := range len(runes) {
		curr := runes[i]
		if i > 0 && unicode.IsUpper(curr) {
			prev := runes[i-1]
			if !unicode.IsUpper(prev) {
				b.WriteByte('_')
			} else if i+1 < len(runes) && !unicode.IsUpper(runes[i+1]) {
				b.WriteByte('_')
			}
		}

		b.WriteRune(unicode.ToLower(curr))
	}

	return b.String()
}
