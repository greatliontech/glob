package glob_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/greatliontech/glob"
)

func TestQuoteExamples(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"plain/path", "plain/path"},
		{"a*b", `a\*b`},
		{"a?[b]{c,d}", `a\?\[b\]\{c\,d\}`},
		{`back\slash`, `back\\slash`},
		{"trailing\\", "trailing\\\\"},
		{"**", `\*\*`},
		{"emoji-🦁/*", `emoji-🦁/\*`},
	}
	for _, c := range cases {
		if got := glob.Quote(c.in); got != c.want {
			t.Fatalf("Quote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// quoteAlphabet mixes ordinary runes, every quotable metacharacter,
// plausible separators, and multibyte runes.
var quoteAlphabet = []rune("ab/\\*?[]{},!-.:@🦁é\x00")

func randomQuoteInput(rng *rand.Rand) string {
	n := rng.Intn(12)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(quoteAlphabet[rng.Intn(len(quoteAlphabet))])
	}
	return b.String()
}

// TestQuoteMatchesExactlyItsInput enforces INV-GLOB-QUOTE-EXACT: over
// random inputs and several separator configurations, the quoted
// pattern compiles, matches its input, and rejects every perturbation
// — append, prepend, rune truncation, and single-rune substitution.
func TestQuoteMatchesExactlyItsInput(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seps := []rune{'/', '\\', ',', '*'}
	for i := 0; i < 500; i++ {
		s := randomQuoteInput(rng)
		for _, sep := range seps {
			p, err := glob.Compile(glob.Quote(s), glob.WithSeparator(sep))
			if err != nil {
				t.Fatalf("Compile(Quote(%q), sep=%q) = %v", s, sep, err)
			}
			if !p.Match(s) {
				t.Fatalf("Quote(%q) does not match its input (sep %q)", s, sep)
			}
			for _, perturbed := range perturbations(rng, s) {
				if perturbed == s {
					continue
				}
				if p.Match(perturbed) {
					t.Fatalf("Quote(%q) also matches %q (sep %q): quoting must be exact", s, perturbed, sep)
				}
			}
		}
	}
}

func perturbations(rng *rand.Rand, s string) []string {
	out := []string{s + "x", "x" + s}
	runes := []rune(s)
	if len(runes) > 0 {
		out = append(out, string(runes[:len(runes)-1]), string(runes[1:]))
		i := rng.Intn(len(runes))
		r := quoteAlphabet[rng.Intn(len(quoteAlphabet))]
		if r != runes[i] {
			mutated := append(append([]rune{}, runes[:i]...), r)
			out = append(out, string(append(mutated, runes[i+1:]...)))
		}
	}
	return out
}

// FuzzQuoteExact fuzzes the same exactness contract under the default
// separator (INV-GLOB-QUOTE-EXACT).
func FuzzQuoteExact(f *testing.F) {
	for _, seed := range []string{"", "a/b", `a\*`, "**", "{a,b}", "x[y]z", "trailing\\"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := glob.Compile(glob.Quote(s))
		if err != nil {
			// The only admissible failure is a resource limit on the
			// (at most doubled) pattern, never a syntax error.
			if len(glob.Quote(s)) <= glob.MaxPatternBytes {
				t.Fatalf("Compile(Quote(%q)) = %v", s, err)
			}
			return
		}
		if !p.Match(s) {
			t.Fatalf("Quote(%q) does not match its input", s)
		}
		if p.Match(s+"x") || p.Match("x"+s) {
			t.Fatalf("Quote(%q) matches a perturbation", s)
		}
	})
}
