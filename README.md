# glob

`glob` compiles path-aware glob patterns and matches them against strings. It
supports component-local wildcards, component-spanning globstars, character
classes, and nested brace alternatives without accessing the filesystem.

```sh
go get github.com/greatliontech/glob
```

## Usage

Compile a pattern once and reuse it:

```go
pattern, err := glob.Compile("{cmd,internal}/**/*.{go,mod}")
if err != nil {
	return err
}

if pattern.Match("internal/net/http.go") {
	// The complete path matched.
}
```

Use `MustCompile` for patterns that are constants under application control:

```go
var goFiles = glob.MustCompile("**/*.go")
```

Compiled patterns are immutable and safe for concurrent use. `Match` performs
no heap allocations. For an input of `N` runes and source pattern of `M` runes,
matching takes `O(N*M)` time and `O(M)` working memory; for a fixed pattern,
time is linear in the input length.

## Compared With Go Regexp

`glob` is a narrower language than [`regexp`](https://pkg.go.dev/regexp): it
provides path-component semantics directly, while an equivalent regular
expression must encode separators, globstar, alternatives, and whole-input
anchors explicitly. Use `regexp` when general regular-expression syntax or
captures are required.

The repository benchmarks each glob against an equivalent compiled, anchored
Go regexp. One representative run produced:

| Case | Glob pattern | Input | glob | regexp | glob advantage |
| --- | --- | --- | ---: | ---: | ---: |
| Literal match | `src/main.go` | `src/main.go` | 4.59 ns/op | 67.0 ns/op | 14.6x |
| Extension match | `*.go` | `main.go` | 16.7 ns/op | 200 ns/op | 12.0x |
| Recursive extension match | `**/*.go` | `cmd/internal/deep/main.go` | 4.75 ns/op | 551 ns/op | 116x |
| Alternatives match | `{cmd,internal}/**/*.{go,mod}` | `internal/net/http/main.go` | 82.2 ns/op | 595 ns/op | 7.2x |
| Unicode class match | `[α-ω]*.go` | `λογος.go` | 60.5 ns/op | 251 ns/op | 4.1x |

Both matchers reported zero allocations in these cases. Results are from Go
1.26.5 on linux/amd64 with an Intel Core Ultra 7 258V and are illustrative, not
an API guarantee. Run the complete matrix on the target system with:

```sh
go test -run '^$' -bench '^BenchmarkMatch$' -benchmem
```

The equivalent regexp sources and all benchmark inputs are in
[`benchmark_test.go`](benchmark_test.go).

## Pattern Language

| Construct | Meaning |
| --- | --- |
| `*` | Zero or more runes within one component |
| `?` | One rune within one component |
| `[abc]` | One listed rune within one component |
| `[a-z]` | One rune in an inclusive range |
| `[!a-z]` | One rune outside the class |
| `**` | Zero or more complete components when it forms a complete component |
| `{go,mod}` | One alternative; groups may be nested and may span components |
| `\` | Quote the next pattern rune, except that an escaped configured separator outside a class remains structural |

Matching always consumes the entire input. Ordinary wildcards never consume
the separator. `**` is a globstar only when it is exactly two unescaped stars
written together without crossing a brace-group boundary and forming a complete
component. Stars in `pre**post`, `***`, and `*{*,x}` behave as ordinary stars
and cannot cross the separator.

Examples with the default `/` separator:

| Pattern | Matches | Does not match |
| --- | --- | --- |
| `*.go` | `main.go` | `cmd/main.go` |
| `**/*.go` | `main.go`, `cmd/main.go` | `main.mod` |
| `a/**/b` | `a/b`, `a/x/y/b` | `a/x` |
| `{cmd,internal}/**/*.go` | `internal/net/http.go` | `pkg/net/http.go` |

Leading, repeated, and trailing separators are significant. Matching is
case-sensitive, performs no Unicode normalization, gives leading dots no
special treatment, and does not clean `.` or `..` components.

See [`docs/specs/glob.md`](docs/specs/glob.md) for the complete language and
error contract.

## Separators

Slash is the default separator on every operating system. Select another
Unicode scalar value at compile time:

```go
hosts := glob.MustCompile("**.example.com", glob.WithSeparator('.'))
fmt.Println(hosts.Match("api.eu.example.com")) // true
```

The configured separator applies to both the pattern and input. Outside a
character class, an escaped configured separator is always structural. Escaping
is required where the separator would otherwise act as pattern syntax at that
position. Callers matching native filesystem paths must choose a separator and
convert paths themselves; the package deliberately does not infer
`os.PathSeparator`.

## Errors And Limits

`Compile` validates the complete pattern and returns a `*glob.CompileError` for
invalid syntax, invalid UTF-8, invalid configuration, or a pattern longer than
`MaxPatternBytes`. `CompileError.Offset` is a byte offset into the source
pattern, or `-1` for configuration errors. Error messages and the exact offset
chosen when more than one construct is malformed are not compatibility
contracts.

Input strings need not contain valid UTF-8. Matching follows Go range-loop
decoding, so each invalid byte is treated as `U+FFFD`.

## Integration

- Compile when loading configuration, retain the resulting `*Pattern`, and
  reject the containing rule if compilation fails.
- Convert or normalize paths before calling `Match` if the application needs
  those semantics. The pattern and input must use the same separator.
- Treat `Match` as whole-path matching. Add `**/` or another explicit pattern
  construct when matching at arbitrary component depth.
- Audit semantics before replacing another glob implementation. This package
  accepts empty patterns, treats `^` as a class literal rather than negation,
  matches dot-prefixed components normally, and supports braces and globstar;
  those choices commonly differ from `path/filepath.Match`, shells, and watcher
  protocols.
- Do not concatenate literal user text into a pattern without escaping it for
  the pattern position where it will appear.
