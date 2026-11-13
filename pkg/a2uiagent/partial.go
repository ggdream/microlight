package a2uiagent

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"
)

func partialMessage(text string, progressive []string) (a2ui.AgentMessage, bool) {
	var empty a2ui.AgentMessage
	healed, ok := closeJSON(text, progressive)
	if !ok {
		return empty, false
	}
	message, err := a2ui.Decode([]byte(healed))
	if err != nil || message.Create != nil || message.CallRenderer != nil || message.AgentResponse != nil {
		return empty, false
	}
	if message.Components != nil {
		index := strings.Index(text, `"components"`)
		if index < 0 {
			return empty, false
		}
		list := text[index+len(`"components"`):]
		start := strings.IndexByte(list, '[')
		if start < 0 {
			return empty, false
		}
		decoder := json.NewDecoder(strings.NewReader(list[start:]))
		_, _ = decoder.Token()
		var closed []a2ui.Component
		for decoder.More() {
			var item a2ui.Component
			if decoder.Decode(&item) != nil {
				break
			}
			closed = append(closed, item)
		}
		if len(closed) == 0 {
			return empty, false
		}
		message.Components.Components = closed
	}
	return message, true
}

func closeJSON(text string, progressive []string) (string, bool) {
	var closing []byte
	inString, escaped := false, false
	start := 0
	for i := 0; i < len(text); i++ {
		char := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case char == '\\':
				escaped = true
			case char == '"':
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString, start = true, i
		case '{':
			closing = append(closing, '}')
		case '[':
			closing = append(closing, ']')
		case '}', ']':
			if len(closing) == 0 || closing[len(closing)-1] != char {
				return "", false
			}
			closing = closing[:len(closing)-1]
		}
	}
	if inString {
		prefix := strings.TrimSpace(text[:start])
		if escaped || !strings.HasSuffix(prefix, ":") {
			return "", false
		}
		prefix = strings.TrimSpace(prefix[:len(prefix)-1])
		index := strings.LastIndexByte(strings.TrimSuffix(prefix, `"`), '"')
		if index < 0 || !slices.Contains(progressive, prefix[index+1:len(prefix)-1]) {
			return "", false
		}
		value := text[start+1:]
		for _, scheme := range []string{"https://", "http://", "data:"} {
			if strings.HasPrefix(value, scheme) {
				return "", false
			}
		}
		text += `"`
	}
	text = strings.TrimSuffix(strings.TrimSpace(text), ",")
	slices.Reverse(closing)
	return text + string(closing), true
}
