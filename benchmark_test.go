package glob_test

import (
	"regexp"
	"runtime"
	"testing"

	"github.com/greatliontech/glob"
)

var benchmarkCases = []struct {
	name    string
	pattern string
	regexp  string
	input   string
	want    bool
}{
	{"literal/match", "src/main.go", `\Asrc/main\.go\z`, "src/main.go", true},
	{"literal/miss", "src/main.go", `\Asrc/main\.go\z`, "src/main.rs", false},
	{"star/match", "*", `\A[^/]*\z`, "main.go", true},
	{"star/miss", "*", `\A[^/]*\z`, "cmd/main.go", false},
	{"globstar/match", "**", `\A(?s:.*)\z`, "cmd/internal/deep/main.go", true},
	{"extension/match", "*.go", `\A[^/]*\.go\z`, "main.go", true},
	{"extension/miss", "*.go", `\A[^/]*\.go\z`, "main.rs", false},
	{"recursive-extension/match", "**/*.go", `\A(?:(?s:.*)/)?[^/]*\.go\z`, "cmd/internal/deep/main.go", true},
	{"recursive-extension/miss", "**/*.go", `\A(?:(?s:.*)/)?[^/]*\.go\z`, "cmd/internal/deep/main.rs", false},
	{"component/match", "src/*/main.go", `\Asrc/[^/]*/main\.go\z`, "src/cmd/main.go", true},
	{"component/miss", "src/*/main.go", `\Asrc/[^/]*/main\.go\z`, "src/cmd/deep/main.go", false},
	{"class/match", "[!a-c]*", `\A[^/a-c][^/]*\z`, "zebra", true},
	{"class/miss", "[!a-c]*", `\A[^/a-c][^/]*\z`, "cobra", false},
	{"alternatives/match", "{cmd,internal}/**/*.{go,mod}", `\A(?:cmd|internal)/(?:(?s:.*)/)?[^/]*\.(?:go|mod)\z`, "internal/net/http/main.go", true},
	{"alternatives/miss", "{cmd,internal}/**/*.{go,mod}", `\A(?:cmd|internal)/(?:(?s:.*)/)?[^/]*\.(?:go|mod)\z`, "pkg/net/http/main.go", false},
	{"unicode/match", "[α-ω]*.go", `\A[α-ω][^/]*\.go\z`, "λογος.go", true},
}

func BenchmarkMatch(b *testing.B) {
	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			compiledGlob := glob.MustCompile(bc.pattern)
			compiledRegexp := regexp.MustCompile(bc.regexp)
			if got := compiledGlob.Match(bc.input); got != bc.want {
				b.Fatalf("glob result = %v, want %v", got, bc.want)
			}
			if got := compiledRegexp.MatchString(bc.input); got != bc.want {
				b.Fatalf("regexp result = %v, want %v", got, bc.want)
			}
			b.Run("glob", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(bc.input)))
				for b.Loop() {
					matchSink = compiledGlob.Match(bc.input)
				}
			})
			b.Run("regexp", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(bc.input)))
				for b.Loop() {
					matchSink = compiledRegexp.MatchString(bc.input)
				}
			})
		})
	}
}

func BenchmarkCompile(b *testing.B) {
	seen := make(map[string]bool)
	for _, bc := range benchmarkCases {
		if seen[bc.pattern] {
			continue
		}
		seen[bc.pattern] = true
		b.Run(bc.name, func(b *testing.B) {
			b.Run("glob", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := glob.Compile(bc.pattern); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("regexp", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := regexp.Compile(bc.regexp); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func BenchmarkMatchParallel(b *testing.B) {
	compiledGlob := glob.MustCompile("{cmd,internal}/**/*.{go,mod}")
	compiledRegexp := regexp.MustCompile(`\A(?:cmd|internal)/(?:(?s:.*)/)?[^/]*\.(?:go|mod)\z`)
	input := "internal/net/http/main.go"
	if !compiledGlob.Match(input) {
		b.Fatal("glob preflight did not match")
	}
	if !compiledRegexp.MatchString(input) {
		b.Fatal("regexp preflight did not match")
	}
	b.Run("glob", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			var matched bool
			for pb.Next() {
				matched = compiledGlob.Match(input)
			}
			runtime.KeepAlive(matched)
		})
	})
	b.Run("regexp", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			var matched bool
			for pb.Next() {
				matched = compiledRegexp.MatchString(input)
			}
			runtime.KeepAlive(matched)
		})
	})
}
