package a2ui

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

var (
	regexUnsupported = regexp.MustCompile(`\(\?[^:=!]|\\[pP]|\\[1-9]`)
	unicodeEscape    = regexp.MustCompile(`\\u(?:\{([0-9A-Fa-f]+)\}|([0-9A-Fa-f]{4}))`)
)

// regexMatch uses RE2 for the consuming expression and for leading lookaheads.
// The catalog's portability limits are checked before matching.
func regexMatch(pattern, value string) (bool, error) {
	if utf8.RuneCountInString(pattern) > 256 || regexUnsupported.MatchString(pattern) {
		return false, fmt.Errorf("regex is outside the A2UI portable subset")
	}
	pattern = unicodeEscape.ReplaceAllString(pattern, `\x{${1}${2}}`)
	if utf8.RuneCountInString(value) > 4096 {
		return false, nil
	}
	if strings.HasPrefix(pattern, "^") {
		pattern = pattern[1:]
		for strings.HasPrefix(pattern, "(?=") || strings.HasPrefix(pattern, "(?!") {
			positive := pattern[2] == '='
			end := regexGroupEnd(pattern)
			if end < 0 {
				return false, fmt.Errorf("invalid regex lookahead")
			}
			matched, err := regexMatch("^"+pattern[3:end], value)
			if err != nil || matched != positive {
				return false, err
			}
			pattern = pattern[end+1:]
		}
		pattern = "^" + pattern
	}
	if err := checkRegexBranches(pattern); err != nil {
		return false, err
	}
	tree, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return false, err
	}
	if _, err = inspectRegex(tree, false); err != nil {
		return false, err
	}
	return regexp.MatchString(pattern, value)
}

func regexGroupEnd(pattern string) int {
	depth := 0
	inClass := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if !inClass {
				depth++
			}
		case ')':
			if !inClass {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

type regexInfo struct {
	first     []rune // Inclusive endpoint pairs.
	nullable  bool
	unbounded bool
}

func inspectRegex(node *syntax.Regexp, repeated bool) (regexInfo, error) {
	info := regexInfo{}
	switch node.Op {
	case syntax.OpLiteral:
		if len(node.Rune) > 0 {
			info.first = []rune{node.Rune[0], node.Rune[0]}
		}
	case syntax.OpCharClass:
		info.first = node.Rune
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		info.first = []rune{0, utf8.MaxRune}
	case syntax.OpCapture:
		return inspectRegex(node.Sub[0], repeated)
	case syntax.OpPlus, syntax.OpStar, syntax.OpQuest, syntax.OpRepeat:
		unbounded := node.Op == syntax.OpPlus || node.Op == syntax.OpStar || (node.Op == syntax.OpRepeat && node.Max < 0)
		if repeated && unbounded {
			return info, fmt.Errorf("NESTED_QUANTIFIER")
		}
		multiple := unbounded || (node.Op == syntax.OpRepeat && node.Max > 1)
		child, err := inspectRegex(node.Sub[0], repeated || multiple)
		if err != nil {
			return info, err
		}
		child.nullable = child.nullable || node.Op == syntax.OpStar || node.Op == syntax.OpQuest || (node.Op == syntax.OpRepeat && node.Min == 0)
		child.unbounded = child.unbounded || unbounded
		return child, nil
	case syntax.OpAlternate:
		for _, sub := range node.Sub {
			child, err := inspectRegex(sub, repeated)
			if err != nil {
				return info, err
			}
			if repeated && (overlap(info.first, child.first) || (len(node.Sub) > 1 && child.nullable)) {
				return info, fmt.Errorf("OVERLAPPING_ALTERNATION")
			}
			info.first = append(info.first, child.first...)
			info.nullable = info.nullable || child.nullable
			info.unbounded = info.unbounded || child.unbounded
		}
	case syntax.OpConcat:
		info.nullable = true
		var prior []rune
		for _, sub := range node.Sub {
			child, err := inspectRegex(sub, repeated)
			if err != nil {
				return info, err
			}
			if info.nullable {
				info.first = append(info.first, child.first...)
			}
			info.nullable = info.nullable && child.nullable
			info.unbounded = info.unbounded || child.unbounded
			if child.unbounded && overlap(prior, child.first) {
				return info, fmt.Errorf("ADJACENT_OVERLAP")
			}
			if !child.nullable {
				prior = nil
			}
			if child.unbounded {
				prior = append(prior, child.first...)
			}
		}
	case syntax.OpNoMatch:
	default:
		info.nullable = true
	}
	return info, nil
}

func overlap(a, b []rune) bool {
	for i := 0; i+1 < len(a); i += 2 {
		for j := 0; j+1 < len(b); j += 2 {
			if a[i] <= b[j+1] && b[j] <= a[i+1] {
				return true
			}
		}
	}
	return false
}

// RE2 merges overlapping character-class alternatives during parsing. Check
// each arm of a repeated group before that information is lost.
func checkRegexBranches(pattern string) error {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] == '[' {
			for i++; i < len(pattern) && pattern[i] != ']'; i++ {
				if pattern[i] == '\\' {
					i++
				}
			}
			continue
		}
		if pattern[i] != '(' {
			continue
		}
		end := regexGroupEnd(pattern[i:])
		if end < 0 || i+end+1 >= len(pattern) {
			continue
		}
		end += i
		if pattern[end+1] != '*' && pattern[end+1] != '+' {
			continue
		}
		body := strings.TrimPrefix(pattern[i+1:end], "?:")
		var seen []rune
		for _, arm := range regexAlternatives(body) {
			tree, err := syntax.Parse(arm, syntax.Perl)
			if err != nil {
				continue
			}
			info, err := inspectRegex(tree, false)
			if err != nil {
				return err
			}
			if overlap(seen, info.first) {
				return fmt.Errorf("OVERLAPPING_ALTERNATION")
			}
			seen = append(seen, info.first...)
		}
	}
	return nil
}

func regexAlternatives(body string) []string {
	var arms []string
	depth, start := 0, 0
	class := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\\' {
			i++
			continue
		}
		if class {
			if c == ']' {
				class = false
			}
			continue
		}
		switch c {
		case '[':
			class = true
		case '(':
			depth++
		case ')':
			depth--
		case '|':
			if depth == 0 {
				arms = append(arms, body[start:i])
				start = i + 1
			}
		}
	}
	return append(arms, body[start:])
}
