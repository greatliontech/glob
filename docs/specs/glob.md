# Path Glob Matching

## Purpose

This document defines a compiled glob language for matching path strings. The
language is path-aware: a configured rune separates components, ordinary
wildcards do not cross component boundaries, and globstar can match multiple
components.

The package matches strings only. It does not access a filesystem, enumerate
directories, resolve links, clean paths, or apply operating-system-specific
path rules.

The language consists of a standard wildcard core and two path-oriented
extensions:

| Construct | Profile |
| --- | --- |
| `*`, `?`, `[...]`, `[!...]`, `\` | Standard wildcard core |
| `**` as a complete component | Globstar extension |
| `{...,...}` | Alternative-group extension |

## Path Model

### GLOB-PATH-001: Whole-path matching

A pattern matches an input only when it consumes the entire input. There is no
implicit leading or trailing wildcard and no substring-search mode.

### GLOB-PATH-002: Configurable separator

Compilation accepts one separator rune. The default separator is `/` on every
operating system. The configured separator has the same meaning in the pattern
and input; no other rune is treated as an equivalent separator.

The separator must be a Unicode scalar value: an integer from `U+0000` through
`U+10FFFF`, excluding surrogate code points `U+D800` through `U+DFFF`.
`U+0000`, Unicode noncharacters, and `U+FFFD` are valid separators.
Configuration rejects an invalid separator rather than substituting the
default.

In a pattern, a separator that does not conflict with pattern syntax is written
directly. A separator that conflicts with syntax at its position is written
with a leading `\` so that the occurrence is structural. An escaped occurrence
of the configured separator outside a character class is structural for every
separator value. The input always contains the separator rune once; pattern
escaping does not alter the input representation. GLOB-SYNTAX-001 defines the
precedence precisely.

For example, when `\` is the separator, the pattern text `a\\b` has two
components and matches the input text `a\b`. The first `\` in the pattern
quotes the second as the configured separator.

Separator selection is explicit and independent of the host operating system.
Callers matching native paths must select or convert their separators as
needed.

### GLOB-PATH-003: Component sequence

A path is a sequence of components separated by the configured separator.
Components may be empty. Consequently, leading, repeated, and trailing
separators are significant. With the default `/` separator:

| Path | Components |
| --- | --- |
| `` | `[""]` |
| `a` | `["a"]` |
| `/a` | `["", "a"]` |
| `a//b/` | `["a", "", "b", ""]` |

Matching does not clean `.` or `..`, combine repeated separators, remove a
trailing separator, or otherwise normalize either operand.

### GLOB-PATH-004: Case and normalization

Matching is case-sensitive and compares Unicode code points exactly. It does
not perform Unicode normalization or locale-sensitive comparison.

A leading `.` has no special status. Wildcards may match dot-prefixed
components, including `.` and `..`.

## Pattern Lexing

### GLOB-SYNTAX-001: Lexical precedence

Pattern text is interpreted with these precedence rules:

1. Inside a character class, character-class syntax has precedence. The
   configured separator is not structural there and is instead excluded from
   the class's accepted runes under GLOB-SYNTAX-006.
2. Outside a character class, `\` quotes the next rune. When that rune is the
   configured separator, the pair denotes a structural separator; otherwise it
   denotes a literal rune.
3. Unescaped `*`, `?`, `[`, and `{` retain their pattern meanings even when one
   is the configured separator. Such a separator must be escaped to be
   structural.
4. Within a brace group, unescaped `,` separates alternatives and unescaped
   `}` closes the group, even when that rune is the configured separator. Such
   a separator must be escaped to be structural.
5. Outside a brace group, `,` is a literal unless it is the configured
   separator, in which case it is structural. An unescaped `}` is structural
   when it is the configured separator and is otherwise invalid.
6. After the preceding rules, an unescaped occurrence of the configured
   separator is structural and every other rune is literal.

Escapes and complete character classes are recognized atomically when finding
brace delimiters. Braces and commas inside a class, and escaped braces or
commas, do not open, close, or divide a brace group.

## Standard Wildcard Core

### GLOB-SYNTAX-002: Component syntax

An ordinary pattern component consists of zero or more of these atoms:

| Atom | Meaning |
| --- | --- |
| literal rune | That rune |
| `?` | Exactly one rune |
| `*` | Zero or more runes |
| character class | Exactly one rune accepted by the class |
| `\` followed by a non-separator rune | The following rune literally |

Ordinary atoms match only within one component and therefore never consume the
configured separator.

An empty pattern component matches only an empty input component. Because `*`
may match zero runes, a component containing `*` may also match an empty input
component.

Brace groups are pattern-level constructs rather than ordinary component
atoms. An alternative may contribute part of a component, a complete
component, or multiple components.

### GLOB-SYNTAX-003: Ordinary star

Within an ordinary component, `*` matches zero or more runes other than the
configured separator. Adjacent ordinary stars are equivalent to one ordinary
star.

A run of stars is globstar only under GLOB-SYNTAX-005. For example, the stars
in `pre**post` are ordinary stars, so that component is equivalent to
`pre*post` and cannot cross the configured separator.

### GLOB-SYNTAX-004: Single-rune wildcard

`?` matches exactly one Unicode code point other than the configured separator.
It does not match an empty component.

### GLOB-SYNTAX-006: Character classes

A character class matches exactly one rune in an ordinary component:

| Form | Meaning |
| --- | --- |
| `[abc]` | `a`, `b`, or `c`, subject to separator exclusion |
| `[a-z]` | Any rune from `a` through `z`, inclusive, except the separator |
| `[!abc]` | Any non-separator component rune except `a`, `b`, and `c` |

Classes may contain multiple literals and ranges. Ranges compare Unicode code
points and may span non-ASCII runes. A range whose upper endpoint precedes its
lower endpoint is invalid.

`!` negates a class only when it is the first unescaped rune after `[`. It is a
literal elsewhere, and `\!` represents a literal in the first position. `^`
has no negation meaning and is always a literal.

Within a class, `\` quotes exactly the next rune. A trailing `\` before the end
of the pattern is invalid if it has no rune to quote. The quoted rune is a
literal class member and remains subject to the configured-separator
prohibition.

Class tokenization occurs before ranges are formed:

- An unescaped `]` is a literal token only when it occurs immediately after the
  opening `[` or optional negation marker; every other unescaped `]` closes the
  class.
- An escaped rune is a literal token.
- An unescaped `-` is a literal token when it is the first or final token in the
  class body. Every other unescaped `-` is a range-operator token.
- Every other rune is a literal token.

Each range-operator token combines its immediately adjacent literal tokens into
one inclusive range. Two range operators cannot be adjacent, and one literal
token cannot be an endpoint of two ranges. Consequently, `[a-cx-z]` contains
two ranges, `[--0]` contains the range `-` through `0`, `[---]` contains the
range `-` through `-`, and `[----]`, `[a-b-c]`, `[a--b]`, and `[a---]` are
invalid.

A class must contain at least one literal or range after an optional negation
marker. Comma has no special class syntax, so with the default separator
`[a,o,u]` also matches `,`.

No character class consumes the configured separator. Listing the separator
explicitly as a literal or range endpoint, escaped or unescaped, is invalid. A
range that spans the separator remains valid but excludes it, and a negated
class also excludes it. A separator rune serving only as class syntax, such as
`-` between range endpoints or `]` closing a class, is not a class member.

### GLOB-SYNTAX-007: Escapes

Outside a character class, `\` causes the next rune to be interpreted
literally, except that an escaped configured separator is structural under
GLOB-PATH-002. A trailing `\` is invalid.

Escaping a rune without special syntax is valid and still denotes that literal
rune. For example, `\a` and `a` have the same meaning.

## Path Extensions

### GLOB-SYNTAX-005: Globstar

A globstar candidate is exactly two contiguous unescaped stars, `**`, written
without crossing a brace-group boundary. The candidate is globstar only when it
alone forms a complete component after alternatives are selected. Globstar
matches zero or more complete input components, including empty components.

Globstar is component-aware rather than an unrestricted character wildcard.
In particular:

- `a/**/b` matches both `a/b` and `a/x/y/b`.
- `**/*.go` matches both `main.go` and `cmd/main.go`.
- `foo/**` matches `foo`, `foo/`, and every path below `foo/`.
- `**/foo` matches `foo`, `a/foo`, and `/foo`.

A component containing any literal, escape, `?`, character class, or a number
of stars other than exactly two is an ordinary component. Thus `***` is
equivalent to `*` and does not cross a separator.

### GLOB-SYNTAX-008: Brace alternatives

`{` and `}` enclose a group of comma-separated alternatives. A group must
contain at least one unescaped comma and therefore at least two alternatives.
Each alternative is a pattern fragment and may contain literals, wildcards,
character classes, configured separators, globstars, and nested brace groups.
Every alternative must consist of complete lexical constructs: a class, escape,
or nested group cannot begin in one alternative or outside the group and end in
another alternative.

Alternatives may be empty. `{,foo}` matches either the empty string or `foo`.
Commas separate alternatives only within the brace group at their own nesting
depth. A comma outside a brace group is a literal unless it is the configured
separator, in which case it is structural. `\{`, `\}`, and `\,` denote literal
brace and comma runes, except when the escaped rune is the configured separator
under GLOB-PATH-002.

Alternatives compose with their surrounding pattern before component
boundaries are determined. Consequently, in `a/{**,x}/b`, the `**` alternative
is a complete globstar component and may consume multiple input components.

Brace boundaries do not synthesize lexical constructs. In particular, the two
stars selected by `*{*,x}` remain adjacent ordinary stars and are equivalent to
one ordinary star; they do not become globstar. A `**` written contiguously
within one alternative may be globstar when that alternative makes it a
complete component.

An unescaped `{` begins a group, and an unescaped `}` ends its current group.
An unclosed group or a group without an alternative comma is invalid. Outside a
group, an unescaped `}` is structural when it is the configured separator and
is otherwise invalid.

### GLOB-SYNTAX-009: Unsupported syntax is literal

The language has no captures, regular-expression escapes, extglob operators,
or implicit case folding. Runes such as `(`, `)`, `+`, `|`, `$`, and `.` are
literals unless they are the configured separator or occur inside another
defined construct.

## Unicode

### GLOB-UNICODE-001: Pattern encoding

A pattern must be valid UTF-8. When compilation reports invalid UTF-8, the
error identifies the byte offset of the first invalid encoding.

### GLOB-UNICODE-002: Input encoding

Matching follows Go's UTF-8 decoding convention: each invalid input byte is
treated as one occurrence of `U+FFFD`. A valid encoded `U+FFFD` denotes the same
code point.

Wildcards and character classes count code points, not bytes or grapheme
clusters. For example, `?` matches `é` encoded as one code point, but it does
not match `e` followed by a combining acute accent because that input contains
two code points.

## Compilation And Errors

### GLOB-COMPILE-001: Compile before matching

Successful compilation produces an immutable matcher reusable for any number
of inputs. Compilation performs all pattern parsing and validation; matching a
compiled pattern does not report syntax errors.

### GLOB-COMPILE-002: Error reporting

Pattern syntax and resource-limit errors identify a byte offset associated with
the malformed or unsupported pattern text. Configuration errors need not carry
a pattern offset. The category, exact offset selection when multiple constructs
are malformed, and human-readable error text are not compatibility contracts.

Compilation rejects at least:

- Invalid UTF-8.
- An invalid separator configuration.
- A trailing escape.
- An unclosed or empty character class.
- A malformed or descending character range.
- The configured separator listed as a class literal or range endpoint.
- An unclosed brace group or unmatched non-separator `}`.
- A brace group without an alternative comma.
- A pattern exceeding documented resource limits.

Compilation returns an error rather than panicking. A panic-on-error compile
operation, when provided, panics exactly when ordinary compilation would
return an error.

### GLOB-COMPILE-003: Resource limits

A pattern may contain at most 4,096 bytes. When compilation reports this limit,
the resource-limit error is associated with byte offset 4,096. GLOB-COMPILE-002
governs patterns having more than one defect.

Every syntactically valid pattern within that byte limit and with a valid
separator configuration must compile successfully. Internal optimization or
generated representation limits cannot cause such a pattern to be rejected.
The byte limit may be raised but not lowered within a released major version.

## Matching Properties

### GLOB-MATCH-001: Determinism

For a compiled pattern and input string, matching always returns the same
boolean result. Results do not depend on prior matches, goroutine scheduling,
or internal optimization selection.

### GLOB-MATCH-002: Concurrent use

A compiled matcher is safe for simultaneous matching by multiple goroutines
without caller-provided synchronization.

### GLOB-MATCH-003: Allocation behavior

Matching a successfully compiled pattern performs no heap allocations. This
property applies equally to matching and non-matching inputs and to concurrent
use.

### GLOB-MATCH-004: Bounded execution

For an input containing `N` decoded code points and source pattern containing
`M` decoded code points, worst-case matching time is bounded by `O(N*M)` and
working memory is bounded by `O(M)`. Matching time is not exponential in input
or source-pattern length. For a fixed compiled pattern, matching time is
therefore linear in input length.

## Behavioral Examples

The following examples are normative:

| Pattern | Input | Matches | Reason |
| --- | --- | --- | --- |
| `` | `` | yes | Both contain one empty component |
| `` | `a` | no | Whole-path matching |
| `*` | `` | yes | Ordinary star may be empty |
| `*` | `a/b` | no | Ordinary star cannot cross `/` |
| `?` | `é` | yes | One Unicode code point |
| `?` | `é` | no | Two Unicode code points |
| `*.go` | `main.go` | yes | One component |
| `*.go` | `cmd/main.go` | no | Ordinary star cannot cross `/` |
| `**` | `cmd/main.go` | yes | Globstar consumes all components |
| `**/*.go` | `main.go` | yes | Globstar consumes zero components |
| `**/*.go` | `cmd/main.go` | yes | Globstar consumes one component |
| `a/**/b` | `a/b` | yes | Globstar consumes zero components |
| `a/**/b` | `a/x/y/b` | yes | Globstar consumes two components |
| `a/**/b` | `a//b` | yes | Globstar consumes one empty component |
| `a/*/b` | `a//b` | yes | Ordinary star matches one empty component |
| `foo/**` | `foo` | yes | Terminal globstar consumes zero components |
| `foo/**` | `foobar` | no | Components must still match |
| `**/foo` | `/foo` | yes | Globstar consumes the leading empty component |
| `/foo` | `foo` | no | Leading empty component is significant |
| `a/` | `a` | no | Trailing empty component is significant |
| `a/` | `a/` | yes | Components are identical |
| `a/***` | `a/x` | yes | `***` is ordinary `*` |
| `a/***` | `a/x/y` | no | `***` is not globstar |
| `[a-c].go` | `b.go` | yes | Inclusive range |
| `[!a-c].go` | `é.go` | yes | Negated class |
| `[]]` | `]` | yes | Initial `]` is a class literal |
| `[-a]` | `-` | yes | Initial `-` is a class literal |
| `[a-]` | `-` | yes | Final `-` is a class literal |
| `[a,o,u]` | `,` | yes | Comma is a class literal |
| `\*.go` | `*.go` | yes | Escaped star is literal |
| `**/*.{go,mod}` | `main.go` | yes | Globstar and `go` alternative |
| `**/*.{go,mod}` | `cmd/go.mod` | yes | Globstar and `mod` alternative |
| `**/*.{go,mod}` | `main.sum` | no | No alternative matches the suffix |
| `{cmd,internal}/**/*.go` | `internal/net/http.go` | yes | Alternatives compose with globstar |
| `a/{**,x}/b` | `a/y/z/b` | yes | The `**` alternative is a globstar component |
| `*{*,x}` | `a/b` | no | Brace boundaries do not synthesize globstar |
| `{,foo}` | `` | yes | Empty alternatives are valid |
| `{a,{b,c}}` | `c` | yes | Alternatives may be nested |
| `*` | `.git` | yes | No hidden-file exception |
| `a/../b` | `a/../b` | yes | `..` is not normalized |

Separator configuration changes component boundaries but not wildcard or
globstar meaning:

| Separator | Pattern | Input | Matches | Reason |
| --- | --- | --- | --- | --- |
| `.` | `*.example.com` | `www.example.com` | yes | `*` consumes one component |
| `.` | `*.example.com` | `api.eu.example.com` | no | `*` cannot cross `.` |
| `.` | `**.example.com` | `api.eu.example.com` | yes | Globstar consumes two components |
| `:` | `a:*` | `a:b:c` | no | Ordinary star cannot cross `:` |
| `:` | `a:**` | `a:b:c` | yes | Globstar consumes two components |
| `\` | `a\\*` | `a\b` | yes | Escaped configured separator is structural |

The following patterns are malformed under the default separator:

| Pattern | Reason |
| --- | --- |
| `[/]` | The separator is an explicit class member |
| `[z-a]` | Descending range |
| `[a-b-c]` | A range endpoint would be shared |
| `[----]` | Adjacent range operators |
| `{a}` | An alternative group requires a comma |
| `{a,b` | Unclosed alternative group |
| `a\` | Trailing escape |

## Project Invariants

### INV-GLOB-SEMANTIC-EQUIVALENCE

Every execution strategy for a successfully compiled pattern accepts exactly
the language defined by this document. Selecting an optimization cannot change
a match result.

INV-GLOB-SEMANTIC-EQUIVALENCE: enforced by `TestExecutionStrategiesMatchProgram`
and `FuzzExecutionStrategiesMatch`.

### INV-GLOB-IMMUTABLE-MATCHER

Compilation is the only operation that constructs matcher state. Matching does
not mutate compiled matcher state.

INV-GLOB-IMMUTABLE-MATCHER: enforced by `TestMatchDoesNotMutatePattern` and
`TestConcurrentMatch`.

### INV-GLOB-BOUNDED-MATCH

The compiled representation supports every valid source pattern admitted by
GLOB-COMPILE-003. Every such pattern has a non-exponential matching strategy
within the documented time and allocation bounds; internal representation
limits cannot reject it or assign it an unbounded matcher.

INV-GLOB-BOUNDED-MATCH: enforced by `FuzzCompiledProgramInvariants`,
`FuzzExecutionStrategiesMatch`, `TestDeterminizationLimits`, and
`TestMatchAllocations`.
