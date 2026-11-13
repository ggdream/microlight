package a2uiagent

import (
	"regexp"
	"strconv"
	"strings"
)

var joinedIdentifier = regexp.MustCompile(`([\pL\pN]+_)\s*\n\s*([\pL\pN_]+)`)
var joinedPath = regexp.MustCompile(`(\$/[a-zA-Z0-9_/]*/)\s*\n\s*([a-zA-Z0-9_]+)([ \t]*[,)\]])`)

// Normalize the string and comment forms Go's scanner does not recognize.
func expressSource(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "<a2ui>") && strings.HasSuffix(input, "</a2ui>") {
		input = strings.TrimSuffix(strings.TrimPrefix(input, "<a2ui>"), "</a2ui>")
	}
	var out strings.Builder
	var plain strings.Builder
	flush := func() {
		value := joinedIdentifier.ReplaceAllString(plain.String(), "${1}${2}")
		out.WriteString(joinedPath.ReplaceAllString(value, "${1}${2}${3}"))
		plain.Reset()
	}
	for i := 0; i < len(input); {
		tail := input[i:]
		switch {
		case strings.HasPrefix(tail, "//"), strings.HasPrefix(tail, "--"), tail[0] == '#':
			end := strings.IndexByte(tail, '\n')
			if end < 0 {
				end = len(tail)
			}
			plain.WriteByte('\n')
			i += end
		case strings.HasPrefix(tail, "/*"):
			end := strings.Index(tail[2:], "*/")
			if end < 0 {
				plain.WriteString(tail)
				i = len(input)
				continue
			}
			plain.WriteByte(' ')
			i += end + 4
		case tail[0] == '"', strings.HasPrefix(tail, `r"`):
			flush()
			raw := tail[0] == 'r'
			if raw {
				i++
				tail = input[i:]
			}
			delimiter := `"`
			if strings.HasPrefix(tail, `"""`) {
				delimiter = `"""`
			}
			start := len(delimiter)
			end := start
			for end < len(tail) && !strings.HasPrefix(tail[end:], delimiter) {
				if !raw && tail[end] == '\\' && end+1 < len(tail) {
					end++
				}
				end++
			}
			if end == len(tail) {
				out.WriteString(tail)
				i = len(input)
				continue
			}
			body := tail[start:end]
			switch {
			case raw:
				out.WriteString(strconv.Quote(body))
			case delimiter == `"""`:
				out.WriteString(strconv.Quote(body))
			default:
				out.WriteByte('"')
				out.WriteString(strings.ReplaceAll(strings.Trim(body, "\r\n"), "\n", `\n`))
				out.WriteByte('"')
			}
			i += end + len(delimiter)
		default:
			plain.WriteByte(input[i])
			i++
		}
	}
	flush()
	return out.String()
}
