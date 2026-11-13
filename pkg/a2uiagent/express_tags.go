package a2uiagent

import (
	"regexp"
	"strings"
)

var outerFence = regexp.MustCompile("(?i)[ \\t]*`{3}(?:a2ui|express)?[ \\t]*(?:\\r?\\n)?$")

func expressClosingTag(previous, pending string) []int {
	source := previous + pending
	limit := len(previous)
	for i := 0; i < len(source); {
		tail := source[i:]
		if match := expressClose.FindStringIndex(tail); match != nil && match[0] == 0 && i >= limit {
			return []int{i - limit, i + match[1] - limit}
		}
		switch {
		case strings.HasPrefix(tail, "/*"):
			end := strings.Index(tail[2:], "*/")
			if end < 0 {
				return nil
			}
			i += end + 4
		case strings.HasPrefix(tail, "//"), strings.HasPrefix(tail, "--"), tail[0] == '#':
			end := strings.IndexByte(tail, '\n')
			if end < 0 {
				end = len(tail)
			}
			if match := expressClose.FindStringIndex(tail[:end]); match != nil && i+match[0] >= limit {
				return []int{i + match[0] - limit, i + match[1] - limit}
			}
			i += end
		case tail[0] == '"' || strings.HasPrefix(tail, `r"`):
			raw := tail[0] == 'r'
			if raw {
				i++
				tail = source[i:]
			}
			delim := `"`
			if strings.HasPrefix(tail, `"""`) {
				delim = `"""`
			}
			end := len(delim)
			for end < len(tail) && !strings.HasPrefix(tail[end:], delim) {
				if !raw && tail[end] == '\\' && end+1 < len(tail) {
					end++
				}
				end++
			}
			if end >= len(tail) {
				return nil
			}
			i += end + len(delim)
		default:
			i++
		}
	}
	return nil
}

func balancedExpress(source string) bool {
	// A synthetic end marker is visible only when no quote or block comment
	// consumes it. Count delimiters after normalizing strings and comments.
	if expressClosingTag("", source+"\n</a2ui>") == nil {
		return false
	}
	source = expressSource(source)
	depth := 0
	quoted, escaped := false, false
	for _, ch := range source {
		if quoted {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				quoted = false
			}
			continue
		}
		switch ch {
		case '"':
			quoted = true
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
	}
	return depth <= 0
}
