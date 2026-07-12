package glob

import (
	"strings"
	"testing"
)

var coreBenchmarkCases = []struct {
	name         string
	pattern      string
	matchInput   string
	failingInput string
}{
	{
		name:         "literal",
		pattern:      "src/main.go",
		matchInput:   "src/main.go",
		failingInput: "src/main.rs",
	},
	{
		name:         "class-star",
		pattern:      "[a-z]*.go",
		matchInput:   "matcher.go",
		failingInput: "Matcher.rs",
	},
	{
		name:         "multiple-classes",
		pattern:      "[a-z][0-9][A-Z][_-][α-ω].go",
		matchInput:   "a7Z_λ.go",
		failingInput: "a7z_λ.go",
	},
	{
		name:         "alternatives",
		pattern:      "{{cmd,internal}/{net,http},{pkg,test}/{api,db}}/*.{go,mod}",
		matchInput:   "internal/http/main.go",
		failingInput: "internal/cache/main.go",
	},
	{
		name:         "dense-active",
		pattern:      strings.Repeat("{,a}", 64) + "b",
		matchInput:   strings.Repeat("a", 64) + "b",
		failingInput: strings.Repeat("a", 64) + "c",
	},
	{
		name:         "globstar-deep",
		pattern:      "root/**/src/**/[a-z]*.go",
		matchInput:   "root/a/b/c/src/d/e/f/matcher.go",
		failingInput: "root/a/b/c/lib/d/e/f/matcher.go",
	},
	{
		name:         "long-input",
		pattern:      "begin*middle*end",
		matchInput:   "begin" + strings.Repeat("a", 2048) + "middle" + strings.Repeat("b", 2048) + "end",
		failingInput: "begin" + strings.Repeat("a", 2048) + "center" + strings.Repeat("b", 2048) + "end",
	},
	{
		name:         "long-pattern",
		pattern:      strings.Repeat("{,a}", 256) + "z",
		matchInput:   "z",
		failingInput: "y",
	},
}

var corePatternSink *Pattern
var coreMatchSink bool

func BenchmarkCoreCompile(b *testing.B) {
	for _, bc := range coreBenchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p, err := compileFallback(bc.pattern, defaultSeparator)
				if err != nil {
					b.Fatal(err)
				}
				corePatternSink = p
			}
		})
	}
}

func BenchmarkCoreMatch(b *testing.B) {
	for _, bc := range coreBenchmarkCases {
		p, err := compileFallback(bc.pattern, defaultSeparator)
		if err != nil {
			b.Fatalf("compileFallback(%q): %v", bc.pattern, err)
		}
		benchCoreMatch(b, bc.name+"/match", p, bc.matchInput, true)
		benchCoreMatch(b, bc.name+"/miss", p, bc.failingInput, false)
	}
}

func BenchmarkExecutionStrategies(b *testing.B) {
	cases := map[string]bool{
		"class-star":       true,
		"multiple-classes": true,
		"alternatives":     true,
		"dense-active":     true,
		"globstar-deep":    true,
		"long-input":       true,
	}
	for _, bc := range coreBenchmarkCases {
		if !cases[bc.name] {
			continue
		}
		dfa, err := compile(bc.pattern, defaultSeparator)
		if err != nil {
			b.Fatalf("Compile(%q): %v", bc.pattern, err)
		}
		if dfa.kind != matcherDFA {
			b.Fatalf("Compile(%q) kind = %v, want DFA", bc.pattern, dfa.kind)
		}
		fallback, err := compileFallback(bc.pattern, defaultSeparator)
		if err != nil {
			b.Fatalf("compileFallback(%q): %v", bc.pattern, err)
		}
		benchStrategyMatch(b, bc.name+"/match", bc.matchInput, true, dfa, fallback)
		benchStrategyMatch(b, bc.name+"/miss", bc.failingInput, false, dfa, fallback)
	}
}

func benchStrategyMatch(b *testing.B, name, input string, want bool, dfa, fallback *Pattern) {
	b.Run(name, func(b *testing.B) {
		if got := dfa.Match(input); got != want {
			b.Fatalf("DFA Match(%q) = %v, want %v", input, got, want)
		}
		if got := fallback.matchProgram(input); got != want {
			b.Fatalf("fallback matchProgram(%q) = %v, want %v", input, got, want)
		}
		b.Run("dfa", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			for b.Loop() {
				coreMatchSink = dfa.Match(input)
			}
		})
		b.Run("nfa", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			for b.Loop() {
				coreMatchSink = fallback.matchProgram(input)
			}
		})
	})
}

func benchCoreMatch(b *testing.B, name string, p *Pattern, input string, want bool) {
	b.Run(name, func(b *testing.B) {
		if got := p.matchProgram(input); got != want {
			b.Fatalf("matchProgram(%q) = %v, want %v", input, got, want)
		}
		b.ReportAllocs()
		b.SetBytes(int64(len(input)))
		for b.Loop() {
			coreMatchSink = p.matchProgram(input)
		}
	})
}
