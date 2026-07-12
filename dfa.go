package glob

import (
	"slices"
	"unicode/utf8"
)

const (
	maxDFAStates      = 256
	maxDFATransitions = 65536
)

type dfaLimits struct {
	states      int
	transitions int
}

var defaultDFALimits = dfaLimits{
	states:      maxDFAStates,
	transitions: maxDFATransitions,
}

type dfaRuneClass struct {
	hi             rune
	representative rune
}

type dfaProgram struct {
	start       uint16
	stride      uint16
	transitions []uint16
	accept      []bool
	classes     []dfaRuneClass
	ascii       [utf8.RuneSelf]uint16
}

func determinize(p *Pattern, limits dfaLimits) *dfaProgram {
	classes := dfaRuneClasses(p)
	stride := len(classes) + 1
	if limits.states < 2 || stride > int(^uint16(0)) || stride > limits.transitions/2 {
		return nil
	}

	d := &dfaProgram{
		stride:      uint16(stride),
		transitions: make([]uint16, stride),
		accept:      []bool{false},
		classes:     classes,
	}
	words := int(p.words)
	states := make([]uint64, words, min(limits.states, 16)*words)
	stateCount := 1

	intern := func(candidate *stateSet) (uint16, bool) {
		if p.empty(candidate) {
			return 0, true
		}
		for state := 1; state < stateCount; state++ {
			at := state * words
			if equalStateSet(states[at:at+words], candidate) {
				return uint16(state), true
			}
		}
		if stateCount >= limits.states || (stateCount+1)*stride > limits.transitions {
			return 0, false
		}
		states = append(states, candidate[:p.words]...)
		d.transitions = append(d.transitions, make([]uint16, stride)...)
		d.accept = append(d.accept, false)
		stateCount++
		return uint16(stateCount - 1), true
	}

	var initial, start stateSet
	initial.add(p.start)
	p.close(&initial)
	p.step(&initial, &start, true, 0)
	startState, ok := intern(&start)
	if !ok {
		return nil
	}
	d.start = startState

	for state := 1; state < stateCount; state++ {
		row := state * stride
		var current stateSet
		copy(current[:p.words], states[state*words:(state+1)*words])
		for symbol := 0; symbol < stride; symbol++ {
			var next stateSet
			if symbol == 0 {
				p.step(&current, &next, true, 0)
			} else {
				p.step(&current, &next, false, classes[symbol-1].representative)
			}
			target, ok := intern(&next)
			if !ok {
				return nil
			}
			d.transitions[row+symbol] = target
			if symbol == 0 && next.has(p.accept) {
				d.accept[state] = true
			}
		}
	}

	for r := rune(0); r < utf8.RuneSelf; r++ {
		if r != p.separator {
			d.ascii[r] = d.symbol(r)
		}
	}
	return d
}

func equalStateSet(a []uint64, b *stateSet) bool {
	for word, value := range a {
		if value != b[word] {
			return false
		}
	}
	return true
}

func dfaRuneClasses(p *Pattern) []dfaRuneClass {
	max := int64(utf8.MaxRune) + 1
	cuts := []int64{0, max, int64(p.separator), int64(p.separator) + 1}
	for _, in := range p.program {
		if in.op == opLiteral {
			cuts = append(cuts, int64(in.r), int64(in.r)+1)
		}
	}
	for _, class := range p.classes {
		for _, interval := range class.ranges {
			cuts = append(cuts, int64(interval.lo), int64(interval.hi)+1)
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)

	classes := make([]dfaRuneClass, 0, len(cuts)-1)
	for i := 0; i+1 < len(cuts); i++ {
		lo, hi := cuts[i], cuts[i+1]-1
		if lo < 0 || lo > int64(utf8.MaxRune) || lo == int64(p.separator) && hi == lo {
			continue
		}
		classes = append(classes, dfaRuneClass{hi: rune(hi), representative: rune(lo)})
	}
	return classes
}

func (d *dfaProgram) symbol(r rune) uint16 {
	lo, hi := 0, len(d.classes)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if r <= d.classes[mid].hi {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return uint16(lo + 1)
}
