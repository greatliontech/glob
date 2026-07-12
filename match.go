package glob

import "math/bits"

const maxStateWords = (maxProgramStates + 63) / 64

type stateSet [maxStateWords]uint64

func (s *stateSet) add(pc uint16) {
	s[pc>>6] |= uint64(1) << (pc & 63)
}

func (s *stateSet) has(pc uint16) bool {
	return s[pc>>6]&(uint64(1)<<(pc&63)) != 0
}

func (s *stateSet) clear(words uint16) {
	for i := uint16(0); i < words; i++ {
		s[i] = 0
	}
}

// Match reports whether input matches the complete pattern.
func (p *Pattern) Match(input string) bool {
	var a, b stateSet
	current, next := &a, &b
	current.add(p.start)
	p.close(current)

	p.step(current, next, true, 0)
	current, next = next, current
	for _, r := range input {
		p.step(current, next, r == p.separator, r)
		current, next = next, current
		if p.empty(current) {
			return false
		}
	}
	p.step(current, next, true, 0)
	return next.has(p.accept)
}

func (p *Pattern) step(current, next *stateSet, boundary bool, r rune) {
	next.clear(p.words)
	for word := uint16(0); word < p.words; word++ {
		active := current[word]
		for active != 0 {
			bit := uint16(bits.TrailingZeros64(active))
			pc := word<<6 | bit
			active &= active - 1
			if int(pc) >= len(p.program) {
				continue
			}
			in := p.program[pc]
			switch in.op {
			case opLiteral:
				if !boundary && r == in.r {
					next.add(in.out)
				}
			case opAny:
				if !boundary {
					next.add(in.out)
				}
			case opClass:
				if !boundary && p.classes[in.class].matches(r) {
					next.add(in.out)
				}
			case opBoundary:
				if boundary {
					next.add(in.out)
				}
			case opStar:
				if !boundary {
					next.add(pc)
				}
			case opGlobstarStart:
				if boundary {
					next.add(in.alt)
				}
			case opGlobstarBody:
				next.add(pc)
			}
		}
	}
	p.close(next)
}

func (p *Pattern) close(states *stateSet) {
	for pc, in := range p.program {
		if !states.has(uint16(pc)) {
			continue
		}
		switch in.op {
		case opJump, opStar, opGlobstarStart, opGlobstarBody:
			states.add(in.out)
		case opSplit:
			states.add(in.out)
			states.add(in.alt)
		}
	}
}

func (p *Pattern) empty(states *stateSet) bool {
	for i := uint16(0); i < p.words; i++ {
		if states[i] != 0 {
			return false
		}
	}
	return true
}
