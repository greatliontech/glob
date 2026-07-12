package glob

import (
	"strings"
	"unicode/utf8"
)

type matcherKind uint8

const (
	matcherProgram matcherKind = iota
	matcherDFA
	matcherLiteral
	matcherAll
	matcherSingleStar
	matcherRecursiveSuffix
)

type specialization struct {
	kind   matcherKind
	prefix string
	suffix string
}

func specialize(root *syntaxNode, separator rune) specialization {
	if root.kind != nodeSequence {
		return specialization{}
	}
	tokens := make([]token, len(root.children))
	for i, child := range root.children {
		if child.kind != nodeToken {
			return specialization{}
		}
		tokens[i] = child.token
	}

	if len(tokens) == 1 && tokens[0].kind == tokenGlobstarCandidate {
		return specialization{kind: matcherAll}
	}
	if recursive, ok := specializeRecursiveSuffix(tokens, separator); ok {
		return recursive
	}
	if separator == utf8.RuneError {
		return specialization{}
	}

	star := -1
	for i, t := range tokens {
		switch t.kind {
		case tokenStar:
			if star >= 0 {
				return specialization{}
			}
			star = i
		case tokenLiteral:
			if t.r == utf8.RuneError {
				return specialization{}
			}
		case tokenSeparator:
		default:
			return specialization{}
		}
	}

	if star < 0 {
		literal, ok := literalTokens(tokens, separator)
		if !ok {
			return specialization{}
		}
		return specialization{kind: matcherLiteral, prefix: literal}
	}
	prefix, ok := literalTokens(tokens[:star], separator)
	if !ok {
		return specialization{}
	}
	suffix, ok := literalTokens(tokens[star+1:], separator)
	if !ok {
		return specialization{}
	}
	return specialization{kind: matcherSingleStar, prefix: prefix, suffix: suffix}
}

func specializeRecursiveSuffix(tokens []token, separator rune) (specialization, bool) {
	if len(tokens) < 3 ||
		tokens[0].kind != tokenGlobstarCandidate ||
		tokens[1].kind != tokenSeparator ||
		tokens[2].kind != tokenStar {
		return specialization{}, false
	}
	var suffix strings.Builder
	for _, t := range tokens[3:] {
		if t.kind != tokenLiteral || t.r == utf8.RuneError {
			return specialization{}, false
		}
		suffix.WriteRune(t.r)
	}
	return specialization{kind: matcherRecursiveSuffix, suffix: suffix.String()}, true
}

func literalTokens(tokens []token, separator rune) (string, bool) {
	var literal strings.Builder
	for _, t := range tokens {
		switch t.kind {
		case tokenLiteral:
			literal.WriteRune(t.r)
		case tokenSeparator:
			literal.WriteRune(separator)
		default:
			return "", false
		}
	}
	return literal.String(), true
}
