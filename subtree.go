package glob

import (
	"strings"
	"unicode/utf8"
)

const (
	surrogateMin            = 0xd800
	surrogateMax            = 0xdfff
	unicodeScalarCount      = (surrogateMin - 0) + (utf8.MaxRune - surrogateMax)
	nonSeparatorScalarCount = unicodeScalarCount - 1
)

// CanMatchInSubtree reports whether there exists at least one non-empty valid
// UTF-8 relative file path rel — consisting of one or more non-empty
// components separated by the configured separator (without a leading
// separator, trailing separator, or consecutive separators) — such that
// p.Match(childPrefix + rel) is true.
//
// Directory-traversal callers pass "" for the root directory and a
// separator-terminated prefix such as "a/b/" for a subdirectory (for "" and
// any separator-terminated childPrefix, returning false also rules out
// non-UTF-8 byte-string rel under GLOB-UNICODE-002); any childPrefix is
// accepted under the same definition. Calling CanMatchInSubtree on a nil
// Pattern returns false.
func (p *Pattern) CanMatchInSubtree(childPrefix string) bool {
	if p == nil {
		return false
	}
	switch p.kind {
	case matcherLiteral:
		return canMatchSubtreeLiteral(childPrefix, p.prefix, p.separator)
	case matcherAll, matcherRecursiveSuffix:
		return true
	case matcherSingleStar:
		return canMatchSubtreeSingleStar(childPrefix, p.prefix, p.suffix, p.separator)
	case matcherDFA:
		return p.canMatchSubtreeDFA(childPrefix)
	default:
		return p.canMatchSubtreeProgram(childPrefix)
	}
}

func canMatchSubtreeLiteral(childPrefix, lit string, sep rune) bool {
	if len(childPrefix) >= len(lit) || !strings.HasPrefix(lit, childPrefix) || !utf8.RuneStart(lit[len(childPrefix)]) {
		return false
	}
	return isValidRelativePath(lit[len(childPrefix):], sep)
}

func canMatchSubtreeSingleStar(childPrefix, prefix, suffix string, sep rune) bool {
	if len(childPrefix) < len(prefix) {
		if !strings.HasPrefix(prefix, childPrefix) || !utf8.RuneStart(prefix[len(childPrefix)]) {
			return false
		}
		remPrefix := prefix[len(childPrefix):]
		first, _ := utf8.DecodeRuneInString(remPrefix)
		if first == sep || hasConsecutiveSeparators(remPrefix, sep) {
			return false
		}
		return suffixAllowsNonSepPrefix(suffix, sep)
	}

	if !strings.HasPrefix(childPrefix, prefix) {
		return false
	}
	afterPrefix := childPrefix[len(prefix):]
	firstSep := strings.IndexRune(afterPrefix, sep)
	if firstSep < 0 {
		return suffixAllowsNonSepPrefix(suffix, sep)
	}

	// afterPrefix contains at least one separator, which * cannot match; any
	// completion must align some non-empty prefix suffix[:k] with the suffix of
	// afterPrefix so that afterPrefix[:len(afterPrefix)-k] precedes firstSep.
	minK := len(afterPrefix) - firstSep
	maxK := min(len(afterPrefix), len(suffix)-1)
	for k := max(1, minK); k <= maxK; k++ {
		if !utf8.RuneStart(suffix[k]) {
			continue
		}
		if afterPrefix[len(afterPrefix)-k:] == suffix[:k] && isValidRelativePath(suffix[k:], sep) {
			return true
		}
	}
	return false
}

func suffixAllowsNonSepPrefix(suffix string, sep rune) bool {
	last, _ := utf8.DecodeLastRuneInString(suffix)
	return last != sep && !hasConsecutiveSeparators(suffix, sep)
}

func isValidRelativePath(s string, sep rune) bool {
	prevSep := true
	for _, r := range s {
		if r == sep {
			if prevSep {
				return false
			}
			prevSep = true
		} else {
			prevSep = false
		}
	}
	return !prevSep
}

func hasConsecutiveSeparators(s string, sep rune) bool {
	prevSep := false
	for _, r := range s {
		if r == sep {
			if prevSep {
				return true
			}
			prevSep = true
		} else {
			prevSep = false
		}
	}
	return false
}

func (p *Pattern) canMatchSubtreeDFA(childPrefix string) bool {
	d := p.dfa
	state := d.start
	stride := int(d.stride)
	for _, r := range childPrefix {
		var symbol uint16
		if r == p.separator {
			symbol = 0
		} else if r < utf8.RuneSelf {
			symbol = d.ascii[r]
		} else {
			symbol = d.symbol(r)
		}
		state = d.transitions[int(state)*stride+int(symbol)]
		if state == 0 {
			return false
		}
	}
	return d.subtree[state]
}

func (p *Pattern) canMatchSubtreeProgram(childPrefix string) bool {
	if len(p.program) == 0 {
		return false
	}
	var a, b stateSet
	current, next := &a, &b
	current.add(p.start)
	p.close(current)

	p.step(current, next, true, 0)
	current, next = next, current
	for _, r := range childPrefix {
		p.step(current, next, r == p.separator, r)
		current, next = next, current
		if p.empty(current) {
			return false
		}
	}
	for w := uint16(0); w < p.words; w++ {
		if current[w]&p.subtree[w] != 0 {
			return true
		}
	}
	return false
}

func computeDFASubtree(d *dfaProgram) []bool {
	stateCount := len(d.accept)
	stride := int(d.stride)
	revSep := make([][]int, stateCount)
	revNonSep := make([][]int, stateCount)
	seenTarget := make([]int, stateCount)

	for prev := 1; prev < stateCount; prev++ {
		row := prev * stride
		if sepTarget := int(d.transitions[row]); sepTarget != 0 {
			revSep[sepTarget] = append(revSep[sepTarget], prev)
		}
		for sym := 1; sym < stride; sym++ {
			target := int(d.transitions[row+sym])
			if target != 0 && seenTarget[target] != prev {
				seenTarget[target] = prev
				revNonSep[target] = append(revNonSep[target], prev)
			}
		}
	}

	// Product node encoding: 2*state + phase, where
	//   phase 0 = awaiting a non-separator rune (start of rel or right after sep),
	//   phase 1 = inside a non-empty component (eligible to accept or read sep).
	vis := make([]bool, 2*stateCount)
	queue := make([]int, 0, 2*stateCount)
	enqueue := func(node int) {
		if !vis[node] {
			vis[node] = true
			queue = append(queue, node)
		}
	}
	for q := 1; q < stateCount; q++ {
		if d.accept[q] {
			enqueue(2*q + 1)
		}
	}
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		q, phase := node>>1, node&1
		if phase == 1 {
			for _, prev := range revNonSep[q] {
				enqueue(2*prev + 0)
				enqueue(2*prev + 1)
			}
		} else {
			for _, prev := range revSep[q] {
				enqueue(2*prev + 1)
			}
		}
	}

	out := make([]bool, stateCount)
	for q := 1; q < stateCount; q++ {
		out[q] = vis[2*q+0]
	}
	return out
}

func computeProgramSubtree(p *Pattern) []uint64 {
	n := len(p.program)
	// 3-phase reverse reachability graph over 3*n nodes (3*pc + phase):
	//   phase 0 = awaiting a non-separator rune (start of rel or right after sep),
	//   phase 1 = inside a non-empty component of rel,
	//   phase 2 = inside the terminal end-of-input boundary step reaching p.accept.
	total := 3 * n
	headEdge := make([]int32, total)
	for i := range headEdge {
		headEdge[i] = -1
	}
	var edgeFrom []int32
	var edgeNext []int32
	addForwardEdge := func(from, to int) {
		idx := int32(len(edgeFrom))
		edgeFrom = append(edgeFrom, int32(from))
		edgeNext = append(edgeNext, headEdge[to])
		headEdge[to] = idx
	}

	for pc, in := range p.program {
		for _, eps := range epsilonTargets(in) {
			for phase := range 3 {
				addForwardEdge(3*pc+phase, 3*eps+phase)
			}
		}
		switch in.op {
		case opLiteral, opAny:
			dst := int(in.out)
			addForwardEdge(3*pc+0, 3*dst+1)
			addForwardEdge(3*pc+1, 3*dst+1)
		case opClass:
			if classMatchesAnyNonSeparator(p.classes[in.class], p.separator) {
				dst := int(in.out)
				addForwardEdge(3*pc+0, 3*dst+1)
				addForwardEdge(3*pc+1, 3*dst+1)
			}
		case opStar:
			addForwardEdge(3*pc+0, 3*pc+1)
			addForwardEdge(3*pc+1, 3*pc+1)
		case opBoundary:
			dst := int(in.out)
			addForwardEdge(3*pc+1, 3*dst+0)
			addForwardEdge(3*pc+1, 3*dst+2)
		case opGlobstarStart:
			dst := int(in.alt)
			addForwardEdge(3*pc+1, 3*dst+0)
			addForwardEdge(3*pc+1, 3*dst+2)
		case opGlobstarBody:
			addForwardEdge(3*pc+0, 3*pc+1)
			addForwardEdge(3*pc+1, 3*pc+1)
			addForwardEdge(3*pc+1, 3*pc+0)
			addForwardEdge(3*pc+1, 3*pc+2)
		}
	}

	vis := make([]bool, total)
	queue := make([]int32, 0, 64)
	seed := int32(3*int(p.accept) + 2)
	vis[seed] = true
	queue = append(queue, seed)
	for qHead := 0; qHead < len(queue); qHead++ {
		cur := queue[qHead]
		for e := headEdge[cur]; e != -1; e = edgeNext[e] {
			prev := edgeFrom[e]
			if !vis[prev] {
				vis[prev] = true
				queue = append(queue, prev)
			}
		}
	}

	out := make([]uint64, p.words)
	for pc := range n {
		if vis[3*pc+0] {
			out[pc>>6] |= uint64(1) << (pc & 63)
		}
	}
	return out
}

func classMatchesAnyNonSeparator(c characterClass, sep rune) bool {
	if !c.negated {
		return true
	}
	var excluded int64
	for _, iv := range c.ranges {
		excluded += countNonSeparatorScalars(iv.lo, iv.hi, sep)
	}
	return excluded < nonSeparatorScalarCount
}

func countNonSeparatorScalars(lo, hi, sep rune) int64 {
	total := int64(hi) - int64(lo) + 1
	sLo := max(lo, rune(surrogateMin))
	sHi := min(hi, rune(surrogateMax))
	if sLo <= sHi {
		total -= int64(sHi) - int64(sLo) + 1
	}
	if sep > lo && sep < hi {
		total--
	}
	return total
}
