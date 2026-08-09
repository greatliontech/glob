package glob

import "strings"

// Quote returns a pattern that matches exactly s and no other input,
// under every valid separator configuration (GLOB-QUOTE-001). Each rune
// with potential pattern syntax — the escape, the wildcards, class and
// group delimiters, and the alternative comma — is preceded by a
// backslash; every other rune is written directly. An escaped rune
// consumes exactly itself whether it is the configured separator
// (structural) or not (literal), so the quoted pattern's match set is
// {s} regardless of separator choice. Compilation limits apply to the
// quoted result as to any pattern.
func Quote(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\', '*', '?', '[', ']', '{', '}', ',':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
