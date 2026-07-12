package glob

import (
	"reflect"
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
		assertProgramInvariants(t, p)
	}
}

func assertProgramInvariants(t testing.TB, p *Pattern) {
	t.Helper()
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

func TestClosureFollowsNewlyActivatedStates(t *testing.T) {
	program := make([]instruction, 67)
	for i := range program {
		program[i].op = opAccept
	}
	program[0] = instruction{op: opJump, out: 1}
	program[1] = instruction{op: opSplit, out: 2, alt: 65}
	program[65] = instruction{op: opJump, out: 66}

	p := &Pattern{program: program, words: 2}
	var states stateSet
	states.add(0)
	p.close(&states)
	for _, pc := range []uint16{0, 1, 2, 65, 66} {
		if !states.has(pc) {
			t.Errorf("closure omitted state %d", pc)
		}
	}
}

func TestMatchDoesNotMutatePattern(t *testing.T) {
	for _, tt := range []struct {
		pattern string
		inputs  []string
	}{
		{"src/main.go", []string{"src/main.go", "src/main.rs"}},
		{"*", []string{"name", "path/name"}},
		{"**", []string{"", "path/name"}},
		{"**/*.go", []string{"path/main.go", "path/main.rs"}},
		{"{cmd,internal}/**/*.{go,mod}", []string{"internal/a/go.mod", "other/a.go"}},
	} {
		t.Run(tt.pattern, func(t *testing.T) {
			p, err := Compile(tt.pattern)
			if err != nil {
				t.Fatalf("Compile(%q): %v", tt.pattern, err)
			}
			want, err := Compile(tt.pattern)
			if err != nil {
				t.Fatalf("second Compile(%q): %v", tt.pattern, err)
			}
			for _, input := range tt.inputs {
				p.Match(input)
				if !reflect.DeepEqual(p, want) {
					t.Fatalf("Match(%q) mutated compiled pattern", input)
				}
			}
		})
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

func FuzzCompiledProgramInvariants(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	f.Add([]byte(strings.Repeat("\x00", MaxPatternBytes)))
	f.Fuzz(func(t *testing.T, data []byte) {
		pattern := generatedValidPattern(data)
		p, err := compileFallback(pattern, defaultSeparator)
		if err != nil {
			t.Fatalf("compileFallback generated valid pattern %q: %v", pattern, err)
		}
		assertProgramInvariants(t, p)
	})
}

func generatedValidPattern(data []byte) string {
	if len(data) > MaxPatternBytes {
		data = data[:MaxPatternBytes]
	}
	fragments := [...]string{
		"a",
		"?",
		"*",
		"[a-z]",
		"[!0-9]",
		`\*`,
		"{x,y}",
		"λ",
		"/**/",
		"{,q}",
		"[α-ω]",
	}
	var pattern strings.Builder
	pattern.Grow(min(len(data), MaxPatternBytes))
	for _, selector := range data {
		fragment := fragments[int(selector)%len(fragments)]
		if pattern.Len()+len(fragment) <= MaxPatternBytes {
			pattern.WriteString(fragment)
		}
	}
	return pattern.String()
}
