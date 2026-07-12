package glob

import (
	"fmt"
	"unicode/utf8"
)

// MaxPatternBytes is the largest pattern accepted by Compile.
const MaxPatternBytes = 4096

const defaultSeparator = '/'

type config struct {
	separator rune
}

// Option configures pattern compilation. Passing a nil Option to [Compile] or
// [MustCompile] is invalid.
type Option func(*config)

// WithSeparator configures separator as the path separator. The default is
// '/'. Separator must be a Unicode scalar value. If multiple separator options
// are supplied, the last one takes effect.
func WithSeparator(separator rune) Option {
	return func(c *config) {
		c.separator = separator
	}
}

// CompileError describes a pattern compilation failure. Offset is a byte
// offset into the pattern, or -1 when the error concerns configuration.
type CompileError struct {
	Offset  int
	Message string
}

func (e *CompileError) Error() string {
	if e.Offset < 0 {
		return "glob: " + e.Message
	}
	return fmt.Sprintf("glob: at byte %d: %s", e.Offset, e.Message)
}

// Pattern is an immutable compiled glob pattern. A Pattern is safe for
// concurrent use by multiple goroutines. Its zero value is invalid; methods
// must be called on a non-nil Pattern returned by successful compilation.
type Pattern struct {
	source    string
	separator rune
	kind      matcherKind
	prefix    string
	suffix    string
	start     uint16
	accept    uint16
	words     uint16
	program   []instruction
	classes   []characterClass
	dfa       *dfaProgram
}

// Compile compiles pattern into an immutable matcher. Pattern must be valid
// UTF-8 and no longer than [MaxPatternBytes]. Options are applied in order.
// Compile returns a [CompileError] for invalid syntax or configuration.
func Compile(pattern string, options ...Option) (*Pattern, error) {
	c := config{separator: defaultSeparator}
	for _, option := range options {
		if option == nil {
			return nil, &CompileError{Offset: -1, Message: "nil compilation option"}
		}
		option(&c)
	}
	if !utf8.ValidRune(c.separator) {
		return nil, &CompileError{Offset: -1, Message: "separator is not a Unicode scalar value"}
	}
	return compile(pattern, c.separator)
}

// MustCompile is like Compile but panics if pattern cannot be compiled.
func MustCompile(pattern string, options ...Option) *Pattern {
	p, err := Compile(pattern, options...)
	if err != nil {
		panic(err)
	}
	return p
}

// String returns the pattern supplied to [Compile].
func (p *Pattern) String() string {
	return p.source
}
