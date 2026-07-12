package glob

import (
	"slices"
	"unicode/utf8"
)

type tokenKind uint8

const (
	tokenLiteral tokenKind = iota
	tokenAny
	tokenClass
	tokenStar
	tokenGlobstarCandidate
	tokenSeparator
)

type token struct {
	kind  tokenKind
	r     rune
	class uint16
}

type nodeKind uint8

const (
	nodeToken nodeKind = iota
	nodeSequence
	nodeAlternative
)

type syntaxNode struct {
	kind     nodeKind
	token    token
	children []*syntaxNode
}

type interval struct {
	lo rune
	hi rune
}

type characterClass struct {
	negated bool
	ranges  []interval
}

func (c characterClass) matches(r rune) bool {
	found := false
	for _, v := range c.ranges {
		if r < v.lo {
			break
		}
		if r <= v.hi {
			found = true
			break
		}
	}
	return found != c.negated
}

type parser struct {
	pattern   string
	separator rune
	pos       int
	classes   []characterClass
}

func compile(pattern string, separator rune) (*Pattern, error) {
	if len(pattern) > MaxPatternBytes {
		return nil, compileError(MaxPatternBytes, "pattern exceeds 4096 bytes")
	}
	if offset := firstInvalidUTF8(pattern); offset >= 0 {
		return nil, compileError(offset, "pattern is not valid UTF-8")
	}

	p := parser{pattern: pattern, separator: separator}
	root, stop, err := p.parseSequence(false)
	if err != nil {
		return nil, err
	}
	if stop != 0 {
		panic("glob: parser stopped at top level")
	}

	lex := []lexInstruction{{op: lexAccept}}
	start := compileLex(root, 0, &lex)
	program, programStart, accept := compileProgram(lex, start)

	return &Pattern{
		source:    pattern,
		separator: separator,
		start:     uint16(programStart),
		accept:    uint16(accept),
		words:     uint16((len(program) + 63) / 64),
		program:   program,
		classes:   p.classes,
	}, nil
}

func firstInvalidUTF8(s string) int {
	for offset := 0; offset < len(s); {
		r, width := utf8.DecodeRuneInString(s[offset:])
		if r == utf8.RuneError && width == 1 {
			return offset
		}
		offset += width
	}
	return -1
}

func compileError(offset int, message string) error {
	return &CompileError{Offset: offset, Message: message}
}

func tokenNode(t token) *syntaxNode {
	return &syntaxNode{kind: nodeToken, token: t}
}

func sequenceNode(children []*syntaxNode) *syntaxNode {
	return &syntaxNode{kind: nodeSequence, children: children}
}

func (p *parser) parseSequence(inGroup bool) (*syntaxNode, rune, error) {
	var children []*syntaxNode
	for p.pos < len(p.pattern) {
		offset := p.pos
		r, width := utf8.DecodeRuneInString(p.pattern[p.pos:])

		if inGroup && (r == ',' || r == '}') {
			return sequenceNode(children), r, nil
		}

		switch r {
		case '\\':
			p.pos += width
			if p.pos == len(p.pattern) {
				return nil, 0, compileError(offset, "trailing escape")
			}
			escaped, escapedWidth := utf8.DecodeRuneInString(p.pattern[p.pos:])
			p.pos += escapedWidth
			if escaped == p.separator {
				children = append(children, tokenNode(token{kind: tokenSeparator}))
			} else {
				children = append(children, tokenNode(token{kind: tokenLiteral, r: escaped}))
			}

		case '[':
			n, err := p.parseClass()
			if err != nil {
				return nil, 0, err
			}
			children = append(children, n)

		case '*':
			count := 0
			for p.pos < len(p.pattern) && p.pattern[p.pos] == '*' {
				p.pos++
				count++
			}
			kind := tokenStar
			if count == 2 {
				kind = tokenGlobstarCandidate
			}
			children = append(children, tokenNode(token{kind: kind}))

		case '?':
			p.pos += width
			children = append(children, tokenNode(token{kind: tokenAny}))

		case '{':
			n, err := p.parseGroup()
			if err != nil {
				return nil, 0, err
			}
			children = append(children, n)

		case '}':
			if r != p.separator {
				return nil, 0, compileError(offset, "unmatched closing brace")
			}
			p.pos += width
			children = append(children, tokenNode(token{kind: tokenSeparator}))

		case ',':
			p.pos += width
			if r == p.separator {
				children = append(children, tokenNode(token{kind: tokenSeparator}))
			} else {
				children = append(children, tokenNode(token{kind: tokenLiteral, r: r}))
			}

		default:
			p.pos += width
			if r == p.separator {
				children = append(children, tokenNode(token{kind: tokenSeparator}))
			} else {
				children = append(children, tokenNode(token{kind: tokenLiteral, r: r}))
			}
		}
	}
	if inGroup {
		return sequenceNode(children), 0, compileError(len(p.pattern), "unclosed alternative group")
	}
	return sequenceNode(children), 0, nil
}

func (p *parser) parseGroup() (*syntaxNode, error) {
	open := p.pos
	p.pos++
	var alternatives []*syntaxNode
	commas := 0
	for {
		alternative, stop, err := p.parseSequence(true)
		if err != nil {
			return nil, err
		}
		alternatives = append(alternatives, alternative)
		switch stop {
		case ',':
			commas++
			p.pos++
		case '}':
			p.pos++
			if commas == 0 {
				return nil, compileError(open, "alternative group requires a comma")
			}
			return &syntaxNode{kind: nodeAlternative, children: alternatives}, nil
		default:
			return nil, compileError(open, "unclosed alternative group")
		}
	}
}

type classToken struct {
	r      rune
	hyphen bool
	offset int
}

func (p *parser) parseClass() (*syntaxNode, error) {
	open := p.pos
	p.pos++
	negated := false
	if p.pos < len(p.pattern) && p.pattern[p.pos] == '!' {
		negated = true
		p.pos++
	}

	var tokens []classToken
	closed := false
	for p.pos < len(p.pattern) {
		offset := p.pos
		r, width := utf8.DecodeRuneInString(p.pattern[p.pos:])
		if r == '\\' {
			p.pos += width
			if p.pos == len(p.pattern) {
				return nil, compileError(offset, "trailing escape in character class")
			}
			r, width = utf8.DecodeRuneInString(p.pattern[p.pos:])
			p.pos += width
			tokens = append(tokens, classToken{r: r, offset: offset})
			continue
		}
		if r == ']' {
			if len(tokens) == 0 {
				p.pos += width
				tokens = append(tokens, classToken{r: r, offset: offset})
				continue
			}
			p.pos += width
			closed = true
			break
		}
		p.pos += width
		tokens = append(tokens, classToken{r: r, hyphen: r == '-', offset: offset})
	}
	if !closed {
		return nil, compileError(open, "unclosed character class")
	}
	if len(tokens) == 0 {
		return nil, compileError(open, "empty character class")
	}

	operators := make([]bool, len(tokens))
	for i, t := range tokens {
		operators[i] = t.hyphen && i != 0 && i != len(tokens)-1
	}
	used := make([]bool, len(tokens))
	var ranges []interval
	for i, operator := range operators {
		if !operator {
			continue
		}
		if operators[i-1] || operators[i+1] || used[i-1] || used[i+1] {
			return nil, compileError(tokens[i].offset, "malformed character range")
		}
		lo, hi := tokens[i-1].r, tokens[i+1].r
		if lo == p.separator || hi == p.separator {
			return nil, compileError(tokens[i].offset, "separator is a character range endpoint")
		}
		if hi < lo {
			return nil, compileError(tokens[i].offset, "descending character range")
		}
		used[i-1], used[i], used[i+1] = true, true, true
		ranges = append(ranges, interval{lo: lo, hi: hi})
	}
	for i, t := range tokens {
		if used[i] {
			continue
		}
		if operators[i] {
			return nil, compileError(t.offset, "malformed character range")
		}
		if t.r == p.separator {
			return nil, compileError(t.offset, "separator is a character class member")
		}
		ranges = append(ranges, interval{lo: t.r, hi: t.r})
	}

	slices.SortFunc(ranges, func(a, b interval) int {
		if a.lo < b.lo {
			return -1
		}
		if a.lo > b.lo {
			return 1
		}
		if a.hi < b.hi {
			return -1
		}
		if a.hi > b.hi {
			return 1
		}
		return 0
	})
	merged := ranges[:0]
	for _, v := range ranges {
		if len(merged) == 0 || v.lo > merged[len(merged)-1].hi+1 {
			merged = append(merged, v)
			continue
		}
		if v.hi > merged[len(merged)-1].hi {
			merged[len(merged)-1].hi = v.hi
		}
	}

	class := len(p.classes)
	if class > int(^uint16(0)) {
		panic("glob: character class bound exceeded")
	}
	p.classes = append(p.classes, characterClass{negated: negated, ranges: merged})
	return tokenNode(token{kind: tokenClass, class: uint16(class)}), nil
}

type lexOpcode uint8

const (
	lexToken lexOpcode = iota
	lexSplit
	lexAccept
)

type lexInstruction struct {
	op       lexOpcode
	token    token
	out, alt int
}

func compileLex(n *syntaxNode, next int, program *[]lexInstruction) int {
	switch n.kind {
	case nodeToken:
		pc := len(*program)
		*program = append(*program, lexInstruction{op: lexToken, token: n.token, out: next})
		return pc
	case nodeSequence:
		start := next
		for i := len(n.children) - 1; i >= 0; i-- {
			start = compileLex(n.children[i], start, program)
		}
		return start
	case nodeAlternative:
		starts := make([]int, len(n.children))
		for i, child := range n.children {
			starts[i] = compileLex(child, next, program)
		}
		start := starts[len(starts)-1]
		for i := len(starts) - 2; i >= 0; i-- {
			pc := len(*program)
			*program = append(*program, lexInstruction{op: lexSplit, out: starts[i], alt: start})
			start = pc
		}
		return start
	default:
		panic("glob: unknown syntax node")
	}
}

type componentState uint8

const (
	componentEmpty componentState = iota
	componentCandidate
	componentOrdinary
	componentStateCount
)

type semanticKind uint8

const (
	semanticLiteral semanticKind = iota
	semanticAny
	semanticClass
	semanticBoundary
	semanticStar
	semanticGlobstar
)

type semanticToken struct {
	kind  semanticKind
	r     rune
	class uint16
}

func transduce(t token, state componentState) ([3]semanticToken, int, componentState) {
	var out [3]semanticToken
	atom := func(kind semanticKind) semanticToken {
		return semanticToken{kind: kind, r: t.r, class: t.class}
	}
	switch t.kind {
	case tokenLiteral, tokenAny, tokenClass:
		kind := semanticLiteral
		if t.kind == tokenAny {
			kind = semanticAny
		} else if t.kind == tokenClass {
			kind = semanticClass
		}
		switch state {
		case componentEmpty:
			out[0], out[1] = atom(semanticBoundary), atom(kind)
			return out, 2, componentOrdinary
		case componentCandidate:
			out[0], out[1], out[2] = atom(semanticBoundary), atom(semanticStar), atom(kind)
			return out, 3, componentOrdinary
		default:
			out[0] = atom(kind)
			return out, 1, componentOrdinary
		}

	case tokenStar:
		if state == componentOrdinary {
			out[0] = atom(semanticStar)
			return out, 1, componentOrdinary
		}
		out[0], out[1] = atom(semanticBoundary), atom(semanticStar)
		return out, 2, componentOrdinary

	case tokenGlobstarCandidate:
		switch state {
		case componentEmpty:
			return out, 0, componentCandidate
		case componentCandidate:
			out[0], out[1] = atom(semanticBoundary), atom(semanticStar)
			return out, 2, componentOrdinary
		default:
			out[0] = atom(semanticStar)
			return out, 1, componentOrdinary
		}

	case tokenSeparator:
		switch state {
		case componentEmpty:
			out[0] = atom(semanticBoundary)
			return out, 1, componentEmpty
		case componentCandidate:
			out[0] = atom(semanticGlobstar)
			return out, 1, componentEmpty
		default:
			return out, 0, componentEmpty
		}
	default:
		panic("glob: unknown token")
	}
}

func finishTransduction(state componentState) ([3]semanticToken, int) {
	var out [3]semanticToken
	switch state {
	case componentEmpty:
		out[0].kind, out[1].kind = semanticBoundary, semanticBoundary
		return out, 2
	case componentCandidate:
		out[0].kind, out[1].kind = semanticGlobstar, semanticBoundary
		return out, 2
	default:
		out[0].kind = semanticBoundary
		return out, 1
	}
}

type opcode uint8

const (
	opJump opcode = iota
	opSplit
	opLiteral
	opAny
	opClass
	opBoundary
	opStar
	opGlobstarStart
	opGlobstarBody
	opAccept
)

type instruction struct {
	op       opcode
	out, alt uint16
	r        rune
	class    uint16
}

const maxProgramStates = 9*MaxPatternBytes + 10

func compileProgram(lex []lexInstruction, lexStart int) ([]instruction, int, int) {
	productCount := len(lex) * int(componentStateCount)
	program := make([]instruction, productCount)
	accept := len(program)
	program = append(program, instruction{op: opAccept})

	product := func(pc int, state componentState) int {
		return pc*int(componentStateCount) + int(state)
	}
	for pc, in := range lex {
		for state := componentEmpty; state < componentStateCount; state++ {
			at := product(pc, state)
			switch in.op {
			case lexSplit:
				program[at] = instruction{
					op:  opSplit,
					out: checkedIndex(product(in.out, state)),
					alt: checkedIndex(product(in.alt, state)),
				}
			case lexToken:
				semantic, n, nextState := transduce(in.token, state)
				destination := product(in.out, nextState)
				start := appendSemantic(program, semantic[:n], destination)
				program = start.program
				program[at] = instruction{op: opJump, out: checkedIndex(start.start)}
			case lexAccept:
				semantic, n := finishTransduction(state)
				start := appendSemantic(program, semantic[:n], accept)
				program = start.program
				program[at] = instruction{op: opJump, out: checkedIndex(start.start)}
			}
		}
	}
	if len(program) > maxProgramStates {
		panic("glob: compiler exceeded proved program bound")
	}
	return orderByEpsilon(program, product(lexStart, componentEmpty), accept)
}

type semanticAppend struct {
	program []instruction
	start   int
}

func appendSemantic(program []instruction, tokens []semanticToken, destination int) semanticAppend {
	start := destination
	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		switch t.kind {
		case semanticLiteral:
			start = len(program)
			program = append(program, instruction{op: opLiteral, out: checkedIndex(destination), r: t.r})
		case semanticAny:
			start = len(program)
			program = append(program, instruction{op: opAny, out: checkedIndex(destination)})
		case semanticClass:
			start = len(program)
			program = append(program, instruction{op: opClass, out: checkedIndex(destination), class: t.class})
		case semanticBoundary:
			start = len(program)
			program = append(program, instruction{op: opBoundary, out: checkedIndex(destination)})
		case semanticStar:
			start = len(program)
			program = append(program, instruction{op: opStar, out: checkedIndex(destination)})
		case semanticGlobstar:
			body := len(program)
			program = append(program, instruction{op: opGlobstarBody, out: checkedIndex(destination)})
			start = len(program)
			program = append(program, instruction{
				op:  opGlobstarStart,
				out: checkedIndex(destination),
				alt: checkedIndex(body),
			})
		}
		destination = start
	}
	return semanticAppend{program: program, start: start}
}

func checkedIndex(i int) uint16 {
	if i < 0 || i > int(^uint16(0)) {
		panic("glob: program index overflow")
	}
	return uint16(i)
}

func orderByEpsilon(program []instruction, start, accept int) ([]instruction, int, int) {
	indegree := make([]int, len(program))
	for _, in := range program {
		for _, target := range epsilonTargets(in) {
			indegree[target]++
		}
	}
	queue := make([]int, 0, len(program))
	for pc, degree := range indegree {
		if degree == 0 {
			queue = append(queue, pc)
		}
	}
	order := make([]int, 0, len(program))
	for len(queue) > 0 {
		pc := queue[0]
		queue = queue[1:]
		order = append(order, pc)
		for _, target := range epsilonTargets(program[pc]) {
			indegree[target]--
			if indegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}
	if len(order) != len(program) {
		panic("glob: epsilon graph contains a cycle")
	}

	remap := make([]uint16, len(program))
	for next, old := range order {
		remap[old] = checkedIndex(next)
	}
	ordered := make([]instruction, len(program))
	for next, old := range order {
		in := program[old]
		switch in.op {
		case opJump, opLiteral, opAny, opClass, opBoundary, opStar, opGlobstarBody:
			in.out = remap[in.out]
		case opSplit, opGlobstarStart:
			in.out = remap[in.out]
			in.alt = remap[in.alt]
		}
		ordered[next] = in
	}
	return ordered, int(remap[start]), int(remap[accept])
}

func epsilonTargets(in instruction) []int {
	switch in.op {
	case opJump, opStar, opGlobstarStart, opGlobstarBody:
		return []int{int(in.out)}
	case opSplit:
		return []int{int(in.out), int(in.alt)}
	default:
		return nil
	}
}
