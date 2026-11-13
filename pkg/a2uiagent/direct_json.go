package a2uiagent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ggdream/microlight/pkg/a2ui"
)

// CompileDirectJSON validates a complete inference block in isolation. Surfaces
// referenced without a create message may have been created on an earlier turn.
func CompileDirectJSON(source string, catalogs []*a2ui.Catalog) ([]a2ui.AgentMessage, error) {
	messages, err := decodeDirectJSON(source)
	if err != nil {
		return nil, err
	}
	var state a2ui.Processor
	for _, catalog := range catalogs {
		if catalog == nil {
			return nil, fmt.Errorf("missing Direct JSON catalog")
		}
		state.RegisterCatalog(catalog)
	}
	mentioned := make(map[string]bool)
	for _, message := range messages {
		if message.Create != nil {
			mentioned[message.Create.ID] = true
		}
	}
	for _, message := range messages {
		var target string
		switch {
		case message.Components != nil:
			target = message.Components.ID
		case message.Data != nil:
			target = message.Data.ID
		case message.CallRenderer != nil:
			target = message.CallRenderer.Surface
		case message.Delete != nil:
			mentioned[message.Delete.ID] = true
		}
		if target != "" && !mentioned[target] && state.Surface(target) == nil {
			var catalog string
			if len(catalogs) == 1 {
				catalog = catalogs[0].ID
			}
			if err := state.Apply(a2ui.AgentMessage{Version: a2ui.Version, Create: &a2ui.CreateSurface{
				SurfaceRef: a2ui.SurfaceRef{ID: target}, Catalog: catalog,
			}}); err != nil {
				return nil, err
			}
		}
		switch {
		case message.CallRenderer != nil:
			err = state.ValidateRendererCall(*message.CallRenderer)
		case message.AgentResponse == nil:
			err = state.Apply(message)
		}
		if err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func decodeDirectJSON(source string) ([]a2ui.AgentMessage, error) {
	if len(source) > maxBlock {
		return nil, fmt.Errorf("A2UI JSON block exceeds %d bytes", maxBlock)
	}
	source = cleanJSON(source)
	if json.Valid([]byte(source)) {
		return a2ui.DecodeBatch([]byte(source))
	}
	// Preserve ordinary JSON and repair only syntax found in model output.
	source = repairJSON(source)
	return a2ui.DecodeBatch([]byte(source))
}

type jsonContainer struct {
	key        string
	object     bool
	expectsKey bool
	data       bool
}

func repairJSON(source string) string {
	var out strings.Builder
	out.Grow(len(source))
	var stack []jsonContainer
	for i := 0; i < len(source); {
		r, width := utf8.DecodeRuneInString(source[i:])
		switch r {
		case '"', '“':
			var frame *jsonContainer
			if len(stack) > 0 {
				frame = &stack[len(stack)-1]
			}
			i += width
			var token strings.Builder
			token.WriteByte('"')
			for i < len(source) {
				ch, n := utf8.DecodeRuneInString(source[i:])
				i += n
				if ch == '\\' && i < len(source) {
					token.WriteByte('\\')
					token.WriteByte(source[i])
					i++
					continue
				}
				if (r == '"' && ch == '"') || (r == '“' && ch == '”') {
					token.WriteByte('"')
					break
				}
				switch ch {
				case '\r', '\n':
					strip := frame != nil && !frame.data && (frame.expectsKey || slices.Contains([]string{
						"id", "component", "componentId", "child", "children", "trigger", "content", "surfaceId", "catalogId", "@catalogId", "functionCallId", "@call",
					}, frame.key))
					if !strip {
						if ch == '\r' {
							token.WriteString(`\r`)
						} else {
							token.WriteString(`\n`)
						}
					}
					for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
						i++
					}
				case '\t':
					token.WriteString(`\t`)
				case '"':
					token.WriteString(`\"`)
				default:
					token.WriteRune(ch)
				}
			}
			if frame != nil && frame.expectsKey {
				_ = json.Unmarshal([]byte(token.String()), &frame.key)
				frame.expectsKey = false
			}
			out.WriteString(token.String())
		case '{', '[':
			frame := jsonContainer{object: r == '{', expectsKey: r == '{'}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				frame.data = parent.data || parent.key == "dataModel" || parent.key == "value"
				if r == '[' {
					frame.key = parent.key
				}
			}
			stack = append(stack, frame)
			out.WriteRune(r)
			i += width
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			out.WriteRune(r)
			i += width
		case ',':
			i += width
			next := strings.TrimLeft(source[i:], " \t\r\n")
			if len(next) == 0 || (next[0] != ']' && next[0] != '}') {
				out.WriteRune(r)
			}
			if len(stack) > 0 {
				stack[len(stack)-1].expectsKey = stack[len(stack)-1].object
			}
		default:
			out.WriteString(source[i : i+width])
			i += width
		}
	}
	return out.String()
}
