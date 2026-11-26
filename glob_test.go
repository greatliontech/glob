package glob_test

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/greatliontech/glob"
)

var regexpOracles = []struct {
	pattern string
	regexp  string
}{
	{"*a?", `\A[^/]*a[^/]\z`},
	{"a/*/b", `\Aa/[^/]*/b\z`},
	{"a/**/b", `\Aa/(?:(?s:.*)/)?b\z`},
	{"**/*.go", `\A(?:(?s:.*)/)?[^/]*\.go\z`},
	{"foo/**", `\Afoo(?:/(?s:.*))?\z`},
	{"**/foo", `\A(?:(?s:.*)/)?foo\z`},
	{"{a,b}/**/*.{go,mod}", `\A(?:a|b)/(?:(?s:.*)/)?[^/]*\.(?:go|mod)\z`},
	{"[!a-c]*", `\A[^/a-c][^/]*\z`},
}

func TestMatchSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"", "", true},
		{"", "a", false},
		{"*", "", true},
		{"*", "a/b", false},
		{"?", "é", true},
		{"?", "e\u0301", false},
		{"*.go", "main.go", true},
		{"*.go", "cmd/main.go", false},
		{"**", "", true},
		{"**", "cmd/main.go", true},
		{"**/*.go", "main.go", true},
		{"**/*.go", "cmd/main.go", true},
		{"**/*.go", "a/b/c/main.go", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/\n/b", true},
		{"a/**/b", "a//b", true},
		{"a/**/b", "a///b", true},
		{"a/*/b", "a/b", false},
		{"a/*/b", "a//b", true},
		{"foo/**", "foo", true},
		{"foo/**", "foo/", true},
		{"foo/**", "foo/x/y", true},
		{"foo/**", "foobar", false},
		{"**/foo", "foo", true},
		{"**/foo", "a/foo", true},
		{"**/foo", "/foo", true},
		{"/foo", "foo", false},
		{"a/", "a", false},
		{"a/", "a/", true},
		{"a/***", "a/x", true},
		{"a/***", "a/x/y", false},
		{"pre**post", "preXXpost", true},
		{"pre**post", "pre/x/post", false},
		{"[a-c].go", "b.go", true},
		{"[!a-c].go", "é.go", true},
		{"[]]", "]", true},
		{"[-a]", "-", true},
		{"[a-]", "-", true},
		{"[a,o,u]", ",", true},
		{`\*.go`, "*.go", true},
		{`\a`, "a", true},
		{"**/*.{go,mod}", "main.go", true},
		{"**/*.{go,mod}", "cmd/go.mod", true},
		{"**/*.{go,mod}", "main.sum", false},
		{"{cmd,internal}/**/*.go", "internal/net/http.go", true},
		{"{src/util,test}/*.go", "src/util/a.go", true},
		{"{src/util,test}/*.go", "test/a.go", true},
		{"a/{**,x}/b", "a/y/z/b", true},
		{"a/{**,x}/b", "a/x/b", true},
		{"*{*,x}", "a/b", false},
		{"{*,x}*", "a/b", false},
		{"**{*,x}", "a/b", false},
		{"{,foo}", "", true},
		{"{,foo}", "foo", true},
		{"{a,{b,c}}", "c", true},
		{"a{,b}", "a", true},
		{"a{,b}", "ab", true},
		{"*", ".git", true},
		{"*", ".", true},
		{"*", "..", true},
		{"a/../b", "a/../b", true},
		{"/", "/", true},
		{"/", "", false},
		{"//", "//", true},
		{"//", "/", false},
		{"*/*", "/", true},
		{"/**/", "/", true},
		{"/**/", "//", true},
		{"/**/", "/x/", true},
		{"/**/", "", false},
		{"{,foo}/bar", "/bar", true},
		{"{,foo}/bar", "foo/bar", true},
		{"[a-cx-z]", "z", true},
		{"[--0]", ".", true},
		{"[---]", "-", true},
		{"[--]", "-", true},
		{"[!abc]", "d", true},
		{`[\!a]`, "!", true},
		{"[a!]", "!", true},
		{"[^a]", "^", true},
		{"[^a]", "b", false},
		{"[]a]", "a", true},
		{"[α-ω]", "λ", true},
		{"{,}", "", true},
		{"{a,b,c}", "c", true},
		{"{a,{b,{c,d}}}", "d", true},
		{"a,{b,c}", "a,c", true},
		{"{*,**}", "a/b", true},
		{"{*,x}", "a/b", false},
		{`\**`, "a/b", false},
		{`*\*`, "a/b", false},
		{"***", "a/b", false},
		{"****", "a/b", false},
		{"a**", "a/x", false},
		{"**a", "x/a", false},
		{"a/{,x}**/b", "a/y/z/b", true},
		{"a/{,x}**/b", "a/xy/z/b", true},
		{"a/**{,x}/b", "a/y/z/b", true},
		{"a/*{*,x}/b", "a/y/z/b", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"/"+tt.input, func(t *testing.T) {
			p, err := glob.Compile(tt.pattern)
			if err != nil {
				t.Fatalf("Compile(%q): %v", tt.pattern, err)
			}
			if got := p.Match(tt.input); got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
			}
		})
	}
	lastWins, err := glob.Compile("*", glob.WithSeparator('/'), glob.WithSeparator('.'))
	if err != nil || !lastWins.Match("a/b") {
		t.Errorf("last separator option did not take effect: pattern=%v err=%v", lastWins, err)
	}
}

func TestConfiguredSeparators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		separator rune
		pattern   string
		input     string
		want      bool
	}{
		{'.', "*.example.com", "www.example.com", true},
		{'.', "*.example.com", "api.eu.example.com", false},
		{'.', "**.example.com", "api.eu.example.com", true},
		{':', "a:*", "a:b:c", false},
		{':', "a:**", "a:b:c", true},
		{'\\', `a\\*`, `a\b`, true},
		{'*', `a\*b`, "a*b", true},
		{'*', "a*b", "axb", true},
		{'?', `a\?b`, "a?b", true},
		{'?', "a?b", "axb", true},
		{'[', `a\[b`, "a[b", true},
		{'{', `a\{b`, "a{b", true},
		{',', "a,b", "a,b", true},
		{',', `a\,b`, "a,b", true},
		{',', `{a\,b,c}`, "a,b", true},
		{'}', "a}b", "a}b", true},
		{'}', `{a\}b,c}`, "a}b", true},
		{'-', "[a-z]", "b", true},
		{']', "[a]", "a", true},
		{'!', "[!a]", "b", true},
		{'.', "a/b", "a/b", true},
		{0, "a\x00b", "a\x00b", true},
		{utf8.RuneError, "a�b", string([]byte{'a', 0xff, 'b'}), true},
	}
	for _, tt := range tests {
		t.Run(string(tt.separator)+"/"+tt.pattern, func(t *testing.T) {
			p, err := glob.Compile(tt.pattern, glob.WithSeparator(tt.separator))
			if err != nil {
				t.Fatalf("Compile(%q, WithSeparator(%q)): %v", tt.pattern, tt.separator, err)
			}
			if got := p.Match(tt.input); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompileErrors(t *testing.T) {
	t.Parallel()
	tests := []string{
		"[/]",
		"[z-a]",
		"[a-b-c]",
		"[----]",
		"{a}",
		"{a,b",
		`a\`,
		"[",
		"[]",
		"[a",
		"[a--b]",
		"[a---]",
		"}",
	}
	for _, pattern := range tests {
		t.Run(pattern, func(t *testing.T) {
			if _, err := glob.Compile(pattern); err == nil {
				t.Fatalf("Compile(%q) succeeded", pattern)
			}
		})
	}

	invalid := string([]byte{'a', 0xff})
	_, err := glob.Compile(invalid)
	var compileErr *glob.CompileError
	if !errors.As(err, &compileErr) || compileErr.Offset != 1 {
		t.Fatalf("Compile(invalid UTF-8) error = %#v, want offset 1", err)
	}

	_, err = glob.Compile(strings.Repeat("a", glob.MaxPatternBytes+1))
	if !errors.As(err, &compileErr) || compileErr.Offset != glob.MaxPatternBytes {
		t.Fatalf("Compile(over limit) error = %#v, want offset %d", err, glob.MaxPatternBytes)
	}

	invalidSeparators := []rune{-1, 0xd800, 0xdfff, utf8.MaxRune + 1}
	for _, separator := range invalidSeparators {
		if _, err := glob.Compile("a", glob.WithSeparator(separator)); err == nil {
			t.Errorf("Compile accepted separator %U", separator)
		}
	}
	if _, err := glob.Compile("a", nil); err == nil {
		t.Error("Compile accepted a nil option")
	}

	if _, err := glob.Compile("[a,o]", glob.WithSeparator(',')); err == nil {
		t.Error("Compile accepted explicit separator in class")
	}
}

func TestPatternLimit(t *testing.T) {
	t.Parallel()
	pattern := strings.Repeat("a", glob.MaxPatternBytes)
	p, err := glob.Compile(pattern)
	if err != nil {
		t.Fatalf("Compile(at limit): %v", err)
	}
	if !p.Match(pattern) {
		t.Fatal("at-limit literal did not match itself")
	}
}

func TestMaximumBraceDepth(t *testing.T) {
	t.Parallel()
	depth := glob.MaxPatternBytes / 3
	pattern := strings.Repeat("{,", depth) + strings.Repeat("}", depth)
	p, err := glob.Compile(pattern)
	if err != nil {
		t.Fatalf("Compile(%d nested groups): %v", depth, err)
	}
	if !p.Match("") {
		t.Fatal("deeply nested empty alternatives did not match empty input")
	}
}

func TestInvalidUTF8Input(t *testing.T) {
	t.Parallel()
	invalid := string([]byte{0xff})
	if !glob.MustCompile("?").Match(invalid) {
		t.Fatal("? did not match one invalid byte as U+FFFD")
	}
	if !glob.MustCompile("�").Match(invalid) {
		t.Fatal("literal U+FFFD did not match one invalid byte")
	}
	if glob.MustCompile("?").Match(string([]byte{0xc0, 0xaf})) {
		t.Fatal("? matched two invalid bytes")
	}
	if !glob.MustCompile("??").Match(string([]byte{0xc0, 0xaf})) {
		t.Fatal("?? did not match two invalid bytes")
	}
}

func TestAgainstRegexpOracle(t *testing.T) {
	t.Parallel()
	alphabet := []rune{'a', 'b', 'c', '/', '.', 'g', 'o'}
	for _, oracle := range regexpOracles {
		p := glob.MustCompile(oracle.pattern)
		re := regexp.MustCompile(oracle.regexp)
		forEachString(alphabet, 5, func(input string) {
			if got, want := p.Match(input), re.MatchString(input); got != want {
				t.Errorf("Match(%q, %q) = %v, regexp %q = %v", oracle.pattern, input, got, oracle.regexp, want)
			}
		})
	}
}

func forEachString(alphabet []rune, max int, fn func(string)) {
	var runes []rune
	var visit func(int)
	visit = func(remaining int) {
		fn(string(runes))
		if remaining == 0 {
			return
		}
		for _, r := range alphabet {
			runes = append(runes, r)
			visit(remaining - 1)
			runes = runes[:len(runes)-1]
		}
	}
	visit(max)
}

var matchSink bool

func TestMatchAllocations(t *testing.T) {
	patterns := []struct {
		pattern string
		input   string
	}{
		{"literal", "literal"},
		{"literal", "different"},
		{"*", "name"},
		{"*", "path/name"},
		{"**", "path/to/name"},
		{"**/*.{go,mod}", "a/b/main.go"},
		{"a/**/[!0-9]?*", "a/x/name"},
		{"**/*.go", "a/b/main.go"},
		{"**/*.go", "a/b/main.sum"},
		{"{cmd,internal}/**/*.{go,mod}", "internal/a/main.go"},
		{strings.Repeat("{,a}", 256) + "b", strings.Repeat("a", 256) + "b"},
	}
	for _, tt := range patterns {
		p := glob.MustCompile(tt.pattern)
		allocs := testing.AllocsPerRun(1000, func() {
			matchSink = p.Match(tt.input)
		})
		if allocs != 0 {
			t.Errorf("Match(%q) allocated %v times", tt.pattern, allocs)
		}
		subtreeAllocs := testing.AllocsPerRun(1000, func() {
			matchSink = p.CanMatchInSubtree(tt.input)
		})
		if subtreeAllocs != 0 {
			t.Errorf("CanMatchInSubtree(%q) allocated %v times", tt.pattern, subtreeAllocs)
		}
	}
}

func TestCanMatchInSubtree(t *testing.T) {
	t.Parallel()
	var nilPattern *glob.Pattern
	if nilPattern.CanMatchInSubtree("") || nilPattern.CanMatchInSubtree("a/") {
		t.Fatal("nil Pattern returned true from CanMatchInSubtree")
	}
	var zeroPattern glob.Pattern
	if zeroPattern.CanMatchInSubtree("") || zeroPattern.CanMatchInSubtree("a/") {
		t.Fatal("zero Pattern returned true from CanMatchInSubtree")
	}

	fullNegatedEmpty := "[!\x00-\ud7ff\ue000-\U0010ffff]"
	nearlyFullNegated := "[!\x00-y{-\ud7ff\ue000-\U0010ffff]"

	tests := []struct {
		pattern     string
		separator   rune
		childPrefix string
		want        bool
	}{
		{"", '/', "", false},
		{"", '/', "a/", false},
		{"/", '/', "", false},
		{"/", '/', "/", false},
		{"a/", '/', "", false},
		{"a/", '/', "a/", false},
		{"a//b", '/', "", false},
		{"a//b", '/', "a/", false},
		{"a//b", '/', "a//", true},
		{"a/b", '/', "", true},
		{"a/b", '/', "a/", true},
		{"a/b", '/', "a/b/", false},
		{"a/b", '/', "b/", false},
		{"a/b", '/', "a", false},
		{"ab", '/', "a", true},
		{"/**", '/', "", false},
		{"/**", '/', "/", true},
		{"/**", '/', "/sub/", true},
		{"**/a/", '/', "", false},
		{"**/a/", '/', "a/", false},
		{"*", '/', "", true},
		{"*", '/', "a/", false},
		{"*.go", '/', "", true},
		{"*.go", '/', "cmd/", false},
		{"a/*.go", '/', "", true},
		{"a/*.go", '/', "a/", true},
		{"a/*.go", '/', "a/sub/", false},
		{"a/*/b/c", '/', "", true},
		{"a/*/b/c", '/', "a/", true},
		{"a/*/b/c", '/', "a/foo/", true},
		{"a/*/b/c", '/', "a/foo/b", false},
		{"a/*/b/c", '/', "a/foo/b/", true},
		{"a/*/b/c", '/', "a/foo/b/c/", false},
		{"a/*b/", '/', "", false},
		{"a/*b/", '/', "a/", false},
		{"a/*b//c", '/', "", false},
		{"a/*b//c", '/', "a/", false},
		{"a//b/*c", '/', "", false},
		{"a/*λ/α", '/', "a/\xbb/", false},
		{"a/*λ/α", '/', "a/xλ/", true},
		{`a\λ*\λc`, 'λ', "aλbλ", true},
		{`a\λ*\λc`, 'λ', "aλbλdλ", false},
		{`[a-z]*\λb`, 'λ', "aλ", true},
		{`[a-z]*\λb`, 'λ', "aλcλ", false},
		{"**", '/', "", true},
		{"**", '/', "any/deep/dir/", true},
		{"**/*.go", '/', "", true},
		{"**/*.go", '/', "any/deep/dir/", true},
		{"a/**/b", '/', "", true},
		{"a/**/b", '/', "a/", true},
		{"a/**/b", '/', "a/x/y/", true},
		{"a/**/b", '/', "other/", false},
		{"{cmd,internal}/**/*.{go,mod}", '/', "", true},
		{"{cmd,internal}/**/*.{go,mod}", '/', "cmd/", true},
		{"{cmd,internal}/**/*.{go,mod}", '/', "internal/net/", true},
		{"{cmd,internal}/**/*.{go,mod}", '/', "vendor/", false},
		{"λόγος/α", '/', string([]byte{0xce}), false},
		{"λόγος/α", '/', "λόγος/", true},
		{fullNegatedEmpty, '/', "", false},
		{nearlyFullNegated, '/', "", true},
		{"a.*.go", '.', "", true},
		{"a.*.go", '.', "a.", true},
		{"a.*.go", '.', "a.b.", true},
		{"a.*.go", '.', "a.b.c.", false},
		{"a.*.go", '.', "x.", false},
		{`a\\*\\c`, '\\', `a\`, true},
		{`a\\*\\c`, '\\', `a\b\`, true},
		{`a\\*\\c`, '\\', `a\b\d\`, false},
	}
	for _, tt := range tests {
		p := glob.MustCompile(tt.pattern, glob.WithSeparator(tt.separator))
		if got := p.CanMatchInSubtree(tt.childPrefix); got != tt.want {
			t.Errorf("Compile(%q, %q).CanMatchInSubtree(%q) = %v, want %v", tt.pattern, tt.separator, tt.childPrefix, got, tt.want)
		}
	}
}

func TestCanMatchInSubtreeExhaustiveOracle(t *testing.T) {
	t.Parallel()
	alphabet := []rune{'a', 'b', '/', '.'}
	var validRels []string
	forEachString(alphabet, 5, func(s string) {
		if s == "" || s[0] == '/' || s[len(s)-1] == '/' || strings.Contains(s, "//") {
			return
		}
		validRels = append(validRels, s)
	})

	patterns := []string{
		"",
		"a",
		"a/b",
		"a/",
		"/a",
		"a//b",
		"*",
		"a/*",
		"a/*/b",
		"a/*b",
		"a*b",
		"**",
		"**/*.a",
		"a/**/b",
		"a/**",
		"**/b",
		"{a,b}/**/*.{a,b}",
		"a/{**,b}/a",
		"[!a]/*",
	}
	for _, pat := range patterns {
		p := glob.MustCompile(pat)
		forEachString(alphabet, 4, func(prefix string) {
			want := false
			for _, rel := range validRels {
				if p.Match(prefix + rel) {
					want = true
					break
				}
			}
			if got := p.CanMatchInSubtree(prefix); got != want {
				t.Fatalf("Compile(%q).CanMatchInSubtree(%q) = %v, brute-force oracle = %v", pat, prefix, got, want)
			}
		})
	}
}

func TestConcurrentMatch(t *testing.T) {
	patterns := []*glob.Pattern{
		glob.MustCompile("{cmd,internal}/**/*.{go,mod}"),
		glob.MustCompile("**/*.go"),
		glob.MustCompile(strings.Repeat("{,a}", 256) + "cmd/main.go"),
	}
	inputs := []string{"cmd/main.go", "internal/a/b/go.mod", "other/main.go", "cmd/main.sum"}
	wants := [][]bool{
		{true, true, false, false},
		{true, false, true, false},
		{true, false, false, false},
	}
	subtrees := []string{"", "cmd/", "vendor/"}
	subtreeWants := [][]bool{
		{true, true, false},
		{true, true, true},
		{true, true, false},
	}
	var wg sync.WaitGroup
	for pattern, p := range patterns {
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range 1000 {
					at := i % len(inputs)
					if got := p.Match(inputs[at]); got != wants[pattern][at] {
						t.Errorf("Match(%q) = %v, want %v", inputs[at], got, wants[pattern][at])
						return
					}
					sat := i % len(subtrees)
					if got := p.CanMatchInSubtree(subtrees[sat]); got != subtreeWants[pattern][sat] {
						t.Errorf("CanMatchInSubtree(%q) = %v, want %v", subtrees[sat], got, subtreeWants[pattern][sat])
						return
					}
				}
			}()
		}
	}
	wg.Wait()
}

func TestMustCompile(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustCompile did not panic")
		}
	}()
	glob.MustCompile("[")
}

func TestMustCompileOptions(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustCompile with invalid option did not panic")
		}
	}()
	glob.MustCompile("a", glob.WithSeparator(0xd800))
}

func FuzzCompileAndMatch(f *testing.F) {
	f.Add("**/*.{go,mod}", "a/b/main.go")
	f.Add("[!a-z]?*", "Xy")
	f.Add("{a,{b,c},d}/**/[0-9-]", "d/x/5")
	f.Add("[]]{,}", "]")
	f.Fuzz(func(t *testing.T, pattern, input string) {
		if len(pattern) > glob.MaxPatternBytes {
			return
		}
		p, err := glob.Compile(pattern)
		if err != nil {
			return
		}
		first := p.Match(input)
		if second := p.Match(input); second != first {
			t.Fatalf("Match(%q, %q) changed from %v to %v", pattern, input, first, second)
		}
	})
}

func FuzzMatchAgainstRegexp(f *testing.F) {
	f.Add(uint8(0), "")
	f.Add(uint8(2), "a/x/y/b")
	f.Add(uint8(2), "a/\n/b")
	f.Add(uint8(3), "cmd/main.go")
	f.Add(uint8(6), "a/x/go.mod")
	f.Fuzz(func(t *testing.T, selected uint8, input string) {
		oracle := regexpOracles[int(selected)%len(regexpOracles)]
		p, err := glob.Compile(oracle.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", oracle.pattern, err)
		}
		re := regexp.MustCompile(oracle.regexp)
		if got, want := p.Match(input), re.MatchString(input); got != want {
			t.Fatalf("Match(%q, %q) = %v, regexp %q = %v", oracle.pattern, input, got, oracle.regexp, want)
		}
	})
}
