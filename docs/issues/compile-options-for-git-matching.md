# Compile-time options, enough of them to match as git does

Lands: when git-go's first plan slots a chunk that matches paths
against git patterns (pathspecs, ignore rules or attribute rules)

git-go (github.com/greatliontech/git-go) must match paths exactly as git
does. Pathspecs, `.gitignore` patterns and `.gitattributes` patterns all
go through git's wildmatch (`wildmatch.c` in git's source). git-go will
use this library for that matching rather than write a second matcher,
for the engine's guarantees: no allocation while matching
(GLOB-MATCH-003) and bounded execution (GLOB-MATCH-004). Git's own
matcher backtracks, with a worst-case cost that has not been established
here, and a git host matches patterns that clients supply.

The language defined here differs from git's. Rather than add a second
fixed language, each difference becomes an option given when a pattern
is compiled, and a named set of options is a preset. Git's behaviour is
one preset; the default language is another. Every option is consumed
by the compiler: a compiled pattern carries no feature switches, so
matching pays nothing for options it does not use, and existing callers
run the loops they run today.

## The unit of matching

Git path names are bytes and need not be valid UTF-8. Today a pattern
must be valid UTF-8 (GLOB-UNICODE-001) and each invalid input byte is
read as U+FFFD (GLOB-UNICODE-002), so a pattern cannot name such a path
and two names differing only in an invalid byte cannot be told apart.

An option selects the unit: code points, as today, or bytes. With
bytes, `?` and a character class match one byte, ranges compare byte
values, and the separator is one byte. This is the one option that
selects a matching loop rather than only shaping what the compiler
emits; the choice is made once, at compilation.

## Syntax options

Each of these is an option of its own. The differences from the default
language are read from `wildmatch.c` at git 2.56.0:

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

- GLOB-MATCH-001 through GLOB-MATCH-004 hold for every combination of
  options as they do today.
- Each option is defined on its own and in combination with the others;
  a combination the language cannot give a meaning to is refused at
  compilation, not left undefined.
- For every pattern and input, the result under the git preset equals
  git's wildmatch result with the corresponding flags. Git's own table
  of cases (`t/t3070-wildmatch.sh`, 190 `match` lines at 2.56.0) is the
  starting test data, and the git binary git-go pins as its oracle
  decides anything the table does not cover.
