package a2uiagent

import (
	"unicode/utf8"
)

type markdownBoundary struct {
	delimiter byte
	length    int
	fenced    bool
	indent    int
	lineStart bool
}

// A tag in a Markdown code span or code fence is ordinary example text.
func (s *Stream) mixedText(final bool) (int, []int, Format) {
	boundary := &s.markdown
	for i := 0; i < len(s.pending); {
		ch := s.pending[i]
		switch {
		case ch == '\n':
			boundary.lineStart, boundary.indent = true, 0
			i++
		case boundary.lineStart && ch == ' ':
			boundary.indent++
			i++
		case ch == '`' || (ch == '~' && boundary.lineStart && boundary.indent <= 3):
			end := i + 1
			for end < len(s.pending) && s.pending[end] == ch {
				end++
			}
			if end == len(s.pending) && !final {
				return i, nil, ""
			}
			length := end - i
			lineFence := boundary.lineStart && boundary.indent <= 3 && length >= 3
			switch {
			case boundary.fenced && lineFence && ch == boundary.delimiter && length >= boundary.length:
				boundary.delimiter, boundary.length, boundary.fenced = 0, 0, false
			case !boundary.fenced && boundary.delimiter == ch && boundary.length == length:
				boundary.delimiter, boundary.length = 0, 0
			case boundary.delimiter == 0 && (lineFence || ch == '`'):
				boundary.delimiter, boundary.length, boundary.fenced = ch, length, lineFence
			}
			boundary.lineStart = false
			i = end
		case ch == '<' && boundary.delimiter == 0:
			tail := s.pending[i:]
			if tag := openTag.FindStringIndex(tail); tag != nil && tag[0] == 0 {
				return i, []int{i, i + tag[1]}, DirectJSON
			}
			if tag := expressOpen.FindStringIndex(tail); tag != nil && tag[0] == 0 {
				return i, []int{i, i + tag[1]}, Express
			}
			if !final && min(holdTag(tail, "<a2ui"), holdTag(tail, "<a2ui-json")) == 0 {
				return i, nil, ""
			}
			boundary.lineStart = false
			i++
		default:
			if !final && !utf8.FullRuneInString(s.pending[i:]) {
				return i, nil, ""
			}
			_, size := utf8.DecodeRuneInString(s.pending[i:])
			boundary.lineStart = false
			i += size
		}
	}
	return len(s.pending), nil, ""
}
