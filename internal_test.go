package glob

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCompiledProgramInvariants(t *testing.T) {
	patterns := []string{
		"",
		"**/*.{go,mod}",
		"{a,b}{c,d}{e,f}/**/[a-z]",
		strings.Repeat("a", MaxPatternBytes),
		strings.Repeat("{,a}", MaxPatternBytes/4),
	}
	for _, pattern := range patterns {
		p, err := compileFallback(pattern, defaultSeparator)
		if err != nil {
			t.Fatalf("Compile(%q): %v", pattern, err)
		}
		if len(p.program) > maxProgramStates {
			t.Fatalf("program has %d states, bound is %d", len(p.program), maxProgramStates)
		}
		for pc, in := range p.program {
			for _, target := range epsilonTargets(in) {
				if target <= pc {
					t.Fatalf("epsilon edge %d -> %d is not forward", pc, target)
				}
			}
		}
	}
}

func TestSpecializationsMatchProgram(t *testing.T) {
	tests := []struct {
		pattern   string
		separator rune
		wantKind  matcherKind
	}{
		{"", '/', matcherLiteral},
		{"src/main.go", '/', matcherLiteral},
		{`\*.go`, '/', matcherLiteral},
		{"*", '/', matcherSingleStar},
		{"*.go", '/', matcherSingleStar},
		{"src/*/main.go", '/', matcherSingleStar},
		{"pre*post", '/', matcherSingleStar},
		{"aba*aba", '/', matcherSingleStar},
		{"λ*ω", '/', matcherSingleStar},
		{"λόγος", '/', matcherLiteral},
		{"**", '/', matcherAll},
		{"**/*.go", '/', matcherRecursiveSuffix},
		{"**/*", '/', matcherRecursiveSuffix},
		{"**/*λ", '/', matcherRecursiveSuffix},
		{"*.go", '.', matcherSingleStar},
		{"a*b", 'λ', matcherSingleStar},
		{`a\**b`, '*', matcherSingleStar},
		{`a\?*b`, '?', matcherSingleStar},
		{`a\\*b`, '\\', matcherSingleStar},
		{"**.*x", '.', matcherRecursiveSuffix},
		{"**�*x", utf8.RuneError, matcherRecursiveSuffix},
		{"�", '/', matcherProgram},
		{"*", utf8.RuneError, matcherProgram},
		{"[a-z]*", '/', matcherProgram},
		{"{a,b}", '/', matcherProgram},
		{"a**b", '/', matcherProgram},
	}
	alphabet := []byte{'a', 'b', '/', '.', 0xff}
	for _, tt := range tests {
		p, err := Compile(tt.pattern, WithSeparator(tt.separator))
		if err != nil {
			t.Fatalf("Compile(%q, %q): %v", tt.pattern, tt.separator, err)
		}
		if p.kind != tt.wantKind {
			t.Errorf("Compile(%q, %q) kind = %v, want %v", tt.pattern, tt.separator, p.kind, tt.wantKind)
		}
		if p.kind == matcherProgram {
			if len(p.program) == 0 {
				t.Errorf("Compile(%q, %q) selected program without instructions", tt.pattern, tt.separator)
			}
		} else if len(p.program) != 0 || len(p.classes) != 0 || p.words != 0 {
			t.Errorf("Compile(%q, %q) specialization retained fallback state", tt.pattern, tt.separator)
		}
		fallback, err := compileFallback(tt.pattern, tt.separator)
		if err != nil {
			t.Fatalf("compileFallback(%q, %q): %v", tt.pattern, tt.separator, err)
		}
		forEachByteString(alphabet, 5, func(input string) {
			if got, want := p.Match(input), fallback.matchProgram(input); got != want {
				t.Errorf("Compile(%q, %q).Match(%q) = %v, program = %v", tt.pattern, tt.separator, input, got, want)
			}
		})
		for _, input := range []string{
			"src/main.go",
			"src/x/main.go",
			"prepost",
			"preXpost",
			"abaaba",
			"abaxaba",
			"λβω",
			"λόγος",
			"main.go",
			"a/b.go",
			"a/bλ",
			"aλb",
			"a*xb",
			"a?xb",
			`a\xb`,
			string([]byte{'a', 0xff, 'x'}),
		} {
			if got, want := p.Match(input), fallback.matchProgram(input); got != want {
				t.Errorf("Compile(%q, %q).Match(%q) = %v, program = %v", tt.pattern, tt.separator, input, got, want)
			}
		}
	}
}

func forEachByteString(alphabet []byte, max int, fn func(string)) {
	var value []byte
	var visit func(int)
	visit = func(remaining int) {
		fn(string(value))
		if remaining == 0 {
			return
		}
		for _, b := range alphabet {
			value = append(value, b)
			visit(remaining - 1)
			value = value[:len(value)-1]
		}
	}
	visit(max)
}
