package util

import (
	"strings"
	"unicode"
)

// CleanName returns a string appropriate for uVM, container, or file names.
//
// Based on [testing.TB.TempDir].
func CleanName(n string) string {
	mapper := func(r rune) rune {
		const allowed = "!#$%&()+,-.=@^_{}~ "
		if unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune(allowed, r) {
			return r
		}
		return -1
	}
	return strings.TrimSpace(strings.Map(mapper, n))
}
