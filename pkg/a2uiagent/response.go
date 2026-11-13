package a2uiagent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"
)

// ResponsePart is either prose or an uncompiled inference payload.
type ResponsePart struct {
	Text    string
	Payload *string
	Final   bool
}

var (
	jsonRecovery    = regexp.MustCompile(`(?m)^[ \t]*(?:\[\s*\{|\{\s*")`)
	expressRecovery = regexp.MustCompile(`(?m)^[ \t]*(?:surface\s*\(|deleteSurface\s*\(|(?:\$[^=\n]+|[\pL_][\pL\pN_]*)\s*=)`)
	lineFence       = regexp.MustCompile("(?m)^[ \\t]*`{3}[^\\n]*(?:\\n|$)")
)

// UnwrapResponse extracts prose and tagged payloads from one complete response.
// A payload with an omitted closing sentinel is marked incomplete.
func UnwrapResponse(format Format, source string) []ResponsePart {
	open, close, start := openTag, closeTag, jsonRecovery
	closing := func(source string) []int { return blockClose(close, source, "") }
	clean := cleanJSON
	if format == Express {
		open, close, start = expressOpen, expressClose, expressRecovery
		closing = func(source string) []int { return expressClosingTag("", source) }
		clean = cleanExpress
	}
	parts := make([]ResponsePart, 0)
	appendText := func(text string) {
		text = strings.TrimSpace(text)
		if strings.Trim(text, "`") != "" {
			parts = append(parts, ResponsePart{Text: text})
		}
	}
	appendPayload := func(text string, final bool) {
		text = strings.Trim(strings.TrimSpace(text), "`")
		// Use the original fenced source when it carries a language name.
		if format == Express {
			text = strings.TrimPrefix(text, "a2ui\n")
			text = strings.TrimPrefix(text, "express\n")
		}
		text = strings.TrimSpace(clean(text))
		parts = append(parts, ResponsePart{Payload: &text, Final: final})
	}
	recovered := false
	for source != "" {
		tag := open.FindStringIndex(source)
		limit := len(source)
		if tag != nil {
			limit = tag[0]
		}
		if !recovered && close.MatchString(source[:limit]) {
			for _, match := range start.FindAllStringIndex(source[:limit], -1) {
				end := closing(source[match[0]:limit])
				if end == nil {
					continue
				}
				head := source[:match[0]]
				fences := lineFence.FindAllStringIndex(head, -1)
				if len(fences)%2 == 1 {
					head = head[:fences[len(fences)-1][0]]
				}
				appendText(head)
				appendPayload(source[match[0]:match[0]+end[0]], true)
				source = strings.TrimLeft(strings.TrimSpace(source[match[0]+end[1]:]), "`")
				recovered = true
				break
			}
			if recovered {
				continue
			}
		}
		if tag == nil {
			appendText(source)
			break
		}
		appendText(strings.TrimRight(strings.TrimSpace(source[:tag[0]]), "`"))
		source = source[tag[1]:]
		end := closing(source)
		if end == nil {
			appendPayload(source, false)
			break
		}
		appendPayload(source[:end[0]], true)
		source = strings.TrimLeft(strings.TrimSpace(source[end[1]:]), "`")
	}
	return parts
}

// WrapResponse emits a complete model-facing response from text and payloads.
func WrapResponse(format Format, parts []ResponsePart) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Payload != nil {
			lines = append(lines, Wrap(format, *part.Payload))
		} else if part.Text != "" {
			lines = append(lines, part.Text)
		}
	}
	return strings.Join(lines, "\n")
}

// HasFormatContent reports whether the response contains a closed payload, or
// any payload when incomplete blocks are allowed.
func HasFormatContent(format Format, source string, complete bool) bool {
	for _, part := range UnwrapResponse(format, source) {
		if part.Payload != nil && (part.Final || !complete) {
			return true
		}
	}
	return false
}

// ParseResponse compiles a complete model response against a renderer state.
func ParseResponse(format Format, source string, catalogs []*a2ui.Catalog, state *a2ui.Processor, unwrapped bool) ([]Part, error) {
	if state == nil {
		state = new(a2ui.Processor)
	}
	for _, catalog := range catalogs {
		if catalog == nil {
			return nil, fmt.Errorf("missing response catalog")
		}
		state.RegisterCatalog(catalog)
	}
	raw := []ResponsePart{{Payload: &source, Final: true}}
	if !unwrapped {
		raw = UnwrapResponse(format, source)
	}
	parts := make([]Part, 0, len(raw))
	for _, item := range raw {
		if item.Payload == nil {
			parts = append(parts, Part{Text: item.Text})
			continue
		}
		if !item.Final {
			return nil, fmt.Errorf("unfinished A2UI response block")
		}
		stream := Stream{State: state, Format: format, Catalogs: catalogs, touched: make(map[string]bool), block: *item.Payload}
		if err := stream.compile(&parts, true); err != nil {
			return nil, err
		}
	}
	return parts, nil
}
