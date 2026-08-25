package stringx

import (
	"unicode"
	"unicode/utf8"
)

// from stdlib strings package
var asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}

// THEORETICALLY equivalent to strings.Fields() except does no mem allocation for slice
// code mostly copied from strings.Fields()
func Parts(s string, destPtr *[]string) {
	setBits := uint8(0)
	for i := 0; i < len(s); i++ {
		setBits |= s[i]
	}

	if setBits >= utf8.RuneSelf {
		partsUnicode(s, destPtr)
		return
	}

	n := len(s)
	i := 0

	for i < n && asciiSpace[s[i]] != 0 {
		i++
	}

	dest := *destPtr
	fieldStart := i
	for i < n {
		if asciiSpace[s[i]] == 0 {
			i++
			continue
		}

		dest = append(dest, s[fieldStart:i])
		i++

		for i < n && asciiSpace[s[i]] != 0 {
			i++
		}
		fieldStart = i
	}

	if fieldStart < n {
		dest = append(dest, s[fieldStart:])
	}

	*destPtr = dest
}

func partsUnicode(s string, destPtr *[]string) {
	dest := *destPtr
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				dest = append(dest, s[start:i])
				start = -1
			}
		} else {
			if start == -1 {
				start = i
			}
		}
	}
	if start >= 0 {
		dest = append(dest, s[start:])
	}

	*destPtr = dest
}
