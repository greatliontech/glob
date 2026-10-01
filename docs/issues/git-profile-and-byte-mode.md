# A byte mode and a git-compatible profile

Lands: when git-go's first plan slots a chunk that matches paths
against git patterns (pathspecs, ignore rules or attribute rules)

git-go (github.com/greatliontech/git-go) must match paths exactly as git
does. Pathspecs, `.gitignore` patterns and `.gitattributes` patterns all
go through git's wildmatch (`wildmatch.c` in git's source). git-go will
use this library for that matching rather than write a second matcher,
for the engine's guarantees: no allocation while matching
(GLOB-MATCH-003) and bounded execution (GLOB-MATCH-004). Git's own
matcher backtracks and can take exponential time, and a git host matches
patterns that clients supply.

The language defined here differs from git's, so two additions are
needed. Both are chosen at compile time. Neither changes the default
language or the matching loops existing callers run.

## Byte mode

Git path names are bytes and need not be valid UTF-8. Today a pattern
must be valid UTF-8 (GLOB-UNICODE-001) and each invalid input byte is
read as U+FFFD (GLOB-UNICODE-002), so a pattern cannot name such a path
and two names differing only in an invalid byte cannot be told apart.

In byte mode the unit of matching is the byte: `?` and a character class
match one byte, ranges compare byte values, and the separator is one
byte.

## Git profile

Differences from the default language, read from `wildmatch.c` at git
2.56.0:

- A class is negated by `^` as well as by `!`.
- POSIX named classes inside a class: `[:alnum:]`, `[:alpha:]`,
  `[:blank:]`, `[:cntrl:]`, `[:digit:]`, `[:graph:]`, `[:lower:]`,
  `[:print:]`, `[:punct:]`, `[:space:]`, `[:upper:]`, `[:xdigit:]`,
  over ASCII.
- `{`, `}` and `,` are literals. There are no alternative groups.
- Optional case folding over ASCII letters (git's `WM_CASEFOLD`).
- Two star behaviours, selected by git's `WM_PATHNAME` flag. Without it,
  `*`, `?` and classes may match `/`, and `**` is the same as `*`. With
  it they do not cross `/`, and `**` is special only as a whole
  component: a leading `**/`, a trailing `/**`, or `/**/`. Elsewhere a
  run of stars acts as one star.
- A trailing `/**` matches what is inside the directory and not the
  directory itself: `foo/**` does not match `foo`. The default language's
  globstar matches zero components, so there `foo/**` matches `foo`.
- A malformed pattern is not an error. An unclosed class or a trailing
  backslash makes the pattern match nothing.
- A class that opens with `[:` but has no closing `:]` is read as an
  ordinary set starting with `[`.

## What must hold

- GLOB-MATCH-001 through GLOB-MATCH-004 hold in byte mode and under the
  git profile as they do today.
- For every pattern and input, the result under the git profile equals
  git's wildmatch result with the corresponding flags. Git's own table
  of cases (`t/t3070-wildmatch.sh`, 190 `match` lines at 2.56.0) is the
  starting test data, and the git binary git-go pins as its oracle
  decides anything the table does not cover.
