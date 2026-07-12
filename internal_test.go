package glob

import (
	"strings"
	"testing"
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
		p, err := Compile(pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", pattern, err)
		}
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
}
