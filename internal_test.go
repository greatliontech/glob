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

func TestExecutionStrategiesMatchProgram(t *testing.T) {
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
		{"�", '/', matcherDFA},
		{"*", utf8.RuneError, matcherDFA},
		{"[a-z]*", '/', matcherDFA},
		{"{a,b}", '/', matcherDFA},
		{"a**b", '/', matcherDFA},
		{"a/**/b", '/', matcherDFA},
		{"[a-z]*", 'm', matcherDFA},
		{"[a-\U0010ffff]", '/', matcherDFA},
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
		switch p.kind {
		case matcherProgram:
			if len(p.program) == 0 {
				t.Errorf("Compile(%q, %q) selected program without instructions", tt.pattern, tt.separator)
			}
			if p.dfa != nil {
				t.Errorf("Compile(%q, %q) fallback retained a DFA", tt.pattern, tt.separator)
			}
		case matcherDFA:
			if p.dfa == nil || len(p.dfa.transitions) == 0 {
				t.Errorf("Compile(%q, %q) selected an empty DFA", tt.pattern, tt.separator)
			}
			if len(p.program) != 0 || len(p.classes) != 0 || p.words != 0 {
				t.Errorf("Compile(%q, %q) DFA retained fallback state", tt.pattern, tt.separator)
			}
		default:
			if p.dfa != nil || len(p.program) != 0 || len(p.classes) != 0 || p.words != 0 {
				t.Errorf("Compile(%q, %q) specialization retained compiled state", tt.pattern, tt.separator)
			}
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
			string(utf8.MaxRune),
			string([]byte{0xff, 0xfe}),
			string([]byte{'a', 0xff, 'x'}),
		} {
			if got, want := p.Match(input), fallback.matchProgram(input); got != want {
				t.Errorf("Compile(%q, %q).Match(%q) = %v, program = %v", tt.pattern, tt.separator, input, got, want)
			}
		}
	}
}

func TestDeterminizationLimits(t *testing.T) {
	p, err := compileFallback("{alpha,beta}/**/[a-z]*.{go,mod}", defaultSeparator)
	if err != nil {
		t.Fatal(err)
	}
	full := determinize(p, defaultDFALimits)
	if full == nil {
		t.Fatal("representative pattern did not determinize")
	}
	exact := dfaLimits{states: len(full.accept), transitions: len(full.transitions)}
	if determinize(p, exact) == nil {
		t.Fatal("determinization failed at exact limits")
	}
	if determinize(p, dfaLimits{states: exact.states - 1, transitions: exact.transitions}) != nil {
		t.Fatal("determinization exceeded state limit")
	}
	if determinize(p, dfaLimits{states: exact.states, transitions: exact.transitions - 1}) != nil {
		t.Fatal("determinization exceeded transition limit")
	}

	pattern := strings.Repeat("{,a}", maxDFAStates) + "b"
	selected, err := Compile(pattern)
	if err != nil {
		t.Fatalf("Compile(over DFA budget): %v", err)
	}
	if selected.kind != matcherProgram {
		t.Fatalf("Compile(over DFA budget) kind = %v, want program fallback", selected.kind)
	}
	if !selected.Match(strings.Repeat("a", maxDFAStates) + "b") {
		t.Fatal("program fallback rejected matching input")
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

func FuzzExecutionStrategiesMatch(f *testing.F) {
	f.Add("a/**/[a-z]*.{go,mod}", "a/x/main.go", uint8(0))
	f.Add("[a-z]*", "amz", uint8(4))
	f.Add("*", string([]byte{0xff}), uint8(9))
	separators := [...]rune{'/', '.', ':', '*', '?', '[', '{', ',', '}', utf8.RuneError, 0, utf8.MaxRune}
	f.Fuzz(func(t *testing.T, pattern, input string, selected uint8) {
		if len(pattern) > MaxPatternBytes {
			return
		}
		separator := separators[int(selected)%len(separators)]
		optimized, optimizedErr := Compile(pattern, WithSeparator(separator))
		fallback, fallbackErr := compileFallback(pattern, separator)
		if (optimizedErr == nil) != (fallbackErr == nil) {
			t.Fatalf("Compile(%q, %q) errors differ: optimized=%v fallback=%v", pattern, separator, optimizedErr, fallbackErr)
		}
		if optimizedErr != nil {
			return
		}
		if got, want := optimized.Match(input), fallback.matchProgram(input); got != want {
			t.Fatalf("Compile(%q, %q).Match(%q) = %v, fallback = %v", pattern, separator, input, got, want)
		}
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
