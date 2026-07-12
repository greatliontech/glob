// Package glob compiles path-aware glob patterns for matching strings.
//
// Patterns match the entire input. By default, slash separates path
// components on every operating system. [WithSeparator] selects another
// separator when compiling a pattern. The package does not access the
// filesystem or clean, normalize, or otherwise modify paths.
//
// The pattern language provides:
//
//   - * to match zero or more runes within one component
//   - ? to match one rune within one component
//   - [...] and [!...] character classes within one component
//   - ** written together as a complete component to match zero or more
//     components
//   - {...,...} alternatives, including nested and empty alternatives
//   - backslash to quote the next pattern rune, except that an escaped
//     configured separator outside a class remains structural
//
// A compiled [Pattern] is immutable and safe for concurrent use. Matching
// performs no heap allocations. For an input of N runes and source pattern of
// M runes, matching takes O(N*M) time and O(M) working memory; for a fixed
// pattern, time is linear in the input length. Compile patterns once and reuse
// them when matching multiple inputs.
package glob
