package a2uiagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ggdream/microlight/pkg/a2ui"
)

const maxBlock = 4 << 20

var (
	openTag      = regexp.MustCompile(`(?i)<a2ui-json(?:\s[^>]*|\s*)?>`)
	expressOpen  = regexp.MustCompile(`(?i)<a2ui(?:\s[^>]*|\s*)?>`)
	expressClose = regexp.MustCompile(`(?i)</a2ui\s*>`)
	closeTag     = regexp.MustCompile(`(?i)</a2ui-json\s*>`)
	fence        = regexp.MustCompile("(?i)^\x60\x60\x60(?:json)?[ \\t]*(?:\\r?\\n)?")
)

type Part struct {
	Text     string
	Messages []a2ui.AgentMessage
	Err      error
}

// Stream accepts arbitrary model-token chunks. A Stream is owned by one goroutine.
type Stream struct {
	State               *a2ui.Processor
	Format              Format
	Catalog             *a2ui.Catalog
	Catalogs            []*a2ui.Catalog
	expressBase         *a2ui.Processor
	expressComponents   map[string]string
	ReceiveRPC          func(a2ui.AgentMessage) error
	Continue            bool
	Unwrapped           bool
	ProgressiveFields   []string
	pending             string
	block               string
	sent                []string
	touched             map[string]bool
	inBlock             bool
	rejected            bool
	textStarted         bool
	done                bool
	expressFencePending bool
	expressStripFence   bool
	blockFormat         Format
	markdown            markdownBoundary
	markdownStarted     bool
}

func (s *Stream) InBlock() bool { return s.inBlock }

func (s *Stream) isExpress() bool {
	return s.Format == Express || (s.Format == Mixed && s.blockFormat == Express)
}

func (s *Stream) Push(chunk string) ([]Part, error) {
	if s.done {
		return nil, fmt.Errorf("A2UI stream is closed")
	}
	if s.Unwrapped && s.Format == Mixed {
		return nil, fmt.Errorf("mixed A2UI stream requires format tags")
	}
	if s.Unwrapped && !s.inBlock {
		s.inBlock = true
		s.touched = make(map[string]bool)
		if s.Format == Express {
			s.startExpress()
		}
	}
	s.pending += chunk
	return s.consume(false)
}

func (s *Stream) Close() ([]Part, error) {
	if s.done {
		return nil, nil
	}
	s.done = true
	return s.consume(true)
}

func Parse(response string, state *a2ui.Processor) ([]Part, error) {
	stream := Stream{State: state, pending: response}
	return stream.Close()
}

func (s *Stream) consume(final bool) ([]Part, error) {
	var parts []Part
	for {
		if !s.inBlock {
			if s.Format == Mixed {
				if !s.markdownStarted {
					s.markdown.lineStart = true
					s.markdownStarted = true
				}
				end, tag, format := s.mixedText(final)
				if tag != nil {
					if before := s.pending[:tag[0]]; before != "" {
						parts = append(parts, Part{Text: before})
					}
					s.pending = s.pending[tag[1]:]
					s.block, s.sent, s.touched, s.inBlock, s.rejected = "", nil, make(map[string]bool), true, false
					s.blockFormat = format
					s.markdown.lineStart = false
					continue
				}
				if end > 0 {
					parts = append(parts, Part{Text: s.pending[:end]})
					s.pending = s.pending[end:]
				}
				return parts, nil
			}
			open, openPrefix := openTag, "<a2ui-json"
			if s.isExpress() {
				open, openPrefix = expressOpen, "<a2ui"
			}
			if s.Format == Express && s.expressStripFence {
				trimmed := strings.TrimLeftFunc(s.pending, unicode.IsSpace)
				if !final && strings.HasPrefix("```", trimmed) && len(trimmed) < 3 {
					return parts, nil
				}
				s.pending = strings.TrimPrefix(trimmed, "```")
				s.expressStripFence = false
			}
			if tag := open.FindStringIndex(s.pending); tag != nil {
				before := s.pending[:tag[0]]
				if s.Format == Express {
					trimmed := outerFence.ReplaceAllString(before, "")
					s.expressStripFence = trimmed != before || s.expressFencePending
					before = trimmed
					if s.expressFencePending && strings.EqualFold(strings.TrimSpace(before), "a2ui") {
						before = ""
					}
					s.expressFencePending = false
					s.text(&parts, before, true)
				} else if s.textStarted && strings.HasPrefix(before, "`") && strings.HasPrefix(strings.TrimPrefix(before, "`"), "\n") {
					parts = append(parts, Part{Text: "`"})
				} else {
					s.text(&parts, before, true)
				}
				s.pending = s.pending[tag[1]:]
				s.block, s.sent, s.touched, s.inBlock, s.rejected = "", nil, make(map[string]bool), true, false
				if s.Format == Express {
					s.startExpress()
				}
				continue
			}
			end := len(s.pending)
			if s.Format == Express {
				trimmed := strings.TrimRightFunc(s.pending, unicode.IsSpace)
				if strings.HasSuffix(trimmed, "```") && !strings.HasPrefix(strings.TrimSpace(trimmed), "```") {
					s.expressFencePending = true
				}
				if s.expressFencePending && strings.HasPrefix(strings.TrimLeftFunc(s.pending, unicode.IsSpace), "```") {
					if match := outerFence.FindStringIndex(s.pending); match != nil && strings.TrimSpace(s.pending[:match[0]]) == "" {
						s.pending = ""
						return parts, nil
					}
					s.pending = strings.TrimPrefix(strings.TrimLeftFunc(s.pending, unicode.IsSpace), "```")
					end = len(s.pending)
				}
			}
			if !final {
				end = holdTag(s.pending, openPrefix)
				end = len(strings.TrimRightFunc(s.pending[:end], unicode.IsSpace))
				if end == len(s.pending) {
					end = len(strings.TrimRight(s.pending[:end], "`"))
				} else if tail := s.pending[end:]; strings.HasPrefix(tail, "<") || strings.Contains(tail, "<a2ui") {
					end = len(strings.TrimRightFunc(s.pending[:end], func(r rune) bool { return r == '`' || unicode.IsSpace(r) }))
				}
				for end > 0 && !utf8.ValidString(s.pending[:end]) {
					end--
				}
			}
			if s.Format == Express && s.expressFencePending {
				end = len(strings.TrimRightFunc(s.pending[:end], unicode.IsSpace))
			}
			s.text(&parts, s.pending[:end], final)
			s.pending = s.pending[end:]
			return parts, nil
		}

		closePrefix := "</a2ui-json"
		tag := blockClose(closeTag, s.pending, s.block)
		if s.isExpress() {
			closePrefix = "</a2ui"
			tag = expressClosingTag(s.block, s.pending)
		}
		end := len(s.pending)
		if tag != nil {
			end = tag[0]
		} else if !final {
			end = holdTag(s.pending, closePrefix)
		}
		if !s.rejected {
			s.block += s.pending[:end]
		}
		s.pending = s.pending[end:]
		if tag != nil {
			s.pending = s.pending[tag[1]-end:]
		}
		finished := tag != nil || final
		if !s.rejected && (s.Format != Mixed || finished || len(s.block) > maxBlock) {
			var err error
			if s.Format == Mixed {
				if tag == nil {
					err = fmt.Errorf("unclosed or oversized A2UI block")
				} else {
					err = s.compileMixed(&parts)
				}
			} else {
				err = s.compile(&parts, finished)
			}
			if err != nil {
				if !s.Continue {
					s.done = true
					return parts, err
				}
				parts = append(parts, Part{Err: err})
				s.rejected = true
				s.block = ""
			}
		}
		if !finished {
			return parts, nil
		}
		s.inBlock = false
		s.textStarted = false
		if final && s.pending == "" {
			return parts, nil
		}
	}
}

func (s *Stream) text(parts *[]Part, text string, boundary bool) {
	if !s.textStarted {
		text = strings.TrimLeftFunc(text, unicode.IsSpace)
		if s.Format != Express {
			text = strings.TrimLeft(text, "`")
		}
	}
	if boundary {
		if s.Format == Express {
			text = outerFence.ReplaceAllString(text, "")
			text = strings.TrimRightFunc(text, unicode.IsSpace)
			if strings.Count(text, "`") == 1 {
				text = strings.TrimSuffix(text, "`")
			}
		} else {
			text = strings.TrimRightFunc(text, func(r rune) bool { return unicode.IsSpace(r) || r == '`' })
		}
	}
	if text != "" && (strings.TrimSpace(strings.Trim(text, "`")) != "" || (s.textStarted && !boundary)) {
		*parts = append(*parts, Part{Text: text})
		s.textStarted = true
	}
}

func holdTag(text, prefix string) int {
	for i := strings.LastIndexByte(text, '<'); i >= 0; {
		tail := strings.ToLower(text[i:])
		if strings.HasPrefix(prefix, tail) || (strings.HasPrefix(tail, prefix) && !strings.Contains(tail, ">") && len(tail) > len(prefix) && unicode.IsSpace(rune(tail[len(prefix)]))) {
			return i
		}
		break
	}
	return len(text)
}

// Ignore sentinel-shaped text inside JSON string literals.
func blockClose(close *regexp.Regexp, pending, previous string) []int {
	matches := close.FindAllStringIndex(pending, -1)
	for _, match := range matches {
		inside, escaped := false, false
		for _, text := range []string{previous, pending[:match[0]]} {
			for _, r := range text {
				switch {
				case escaped:
					escaped = false
				case inside && r == '\\':
					escaped = true
				case r == '"':
					inside = !inside
				}
			}
		}
		if !inside {
			return match
		}
	}
	return nil
}

func (s *Stream) compile(parts *[]Part, final bool) error {
	if len(s.block) > maxBlock {
		return fmt.Errorf("A2UI JSON block exceeds %d bytes", maxBlock)
	}
	if s.isExpress() {
		return s.compileExpress(parts, final)
	}
	source := cleanJSON(s.block)
	if source == "" && final {
		return fmt.Errorf("empty A2UI JSON block")
	}
	var messages []a2ui.AgentMessage
	var err error
	if final {
		messages, err = decodeDirectJSON(source)
		if err != nil {
			return err
		}
	} else {
		messages, err = completeMessages(source, s.ProgressiveFields)
	}
	parseErr := err
	if s.State == nil {
		s.State = new(a2ui.Processor)
	}
	if final && len(s.sent) == 0 {
		allState := true
		for _, message := range messages {
			if message.CallRenderer != nil || message.AgentResponse != nil {
				allState = false
				break
			}
		}
		if allState {
			if err := s.State.ApplyBatch(messages); err != nil {
				return err
			}
			if len(messages) != 0 {
				*parts = append(*parts, Part{Messages: messages})
			}
			return nil
		}
	}
	var delta []a2ui.AgentMessage
	for i, message := range messages {
		data, _ := json.Marshal(message)
		key := string(data)
		if i < len(s.sent) && s.sent[i] == key {
			continue
		}
		if message.CallRenderer != nil || message.AgentResponse != nil {
			err = nil
			if message.CallRenderer != nil {
				err = s.State.ValidateRendererCall(*message.CallRenderer)
			}
			if err == nil && s.ReceiveRPC != nil {
				err = s.ReceiveRPC(message)
			}
		} else {
			if i < len(s.sent) && message.Create != nil {
				return fmt.Errorf("stream attempted to replace a created surface")
			}
			err = s.State.Apply(message)
		}
		if err != nil {
			if len(delta) != 0 {
				*parts = append(*parts, Part{Messages: delta})
			}
			return err
		}
		switch {
		case message.Create != nil:
			s.touched[message.Create.ID] = true
		case message.Components != nil:
			s.touched[message.Components.ID] = true
		}
		if i < len(s.sent) {
			s.sent[i] = key
		} else {
			s.sent = append(s.sent, key)
		}
		delta = append(delta, message)
	}
	if len(delta) != 0 {
		*parts = append(*parts, Part{Messages: delta})
	}
	if parseErr != nil {
		return parseErr
	}
	if final {
		for id := range s.touched {
			if surface := s.State.Surface(id); surface != nil {
				if err := surface.ValidateReachable(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func cleanJSON(source string) string {
	source = strings.Trim(strings.TrimSpace(source), "`")
	// Trimming backticks also removes a leading fence's delimiter.
	if strings.HasPrefix(strings.ToLower(source), "json") {
		source = strings.TrimSpace(source[len("json"):])
	}
	source = fence.ReplaceAllString(source, "")
	return strings.TrimSpace(source)
}

func completeMessages(source string, progressive []string) ([]a2ui.AgentMessage, error) {
	var messages []a2ui.AgentMessage
	if source == "" {
		return nil, nil
	}
	if source[0] != '[' {
		if json.Valid([]byte(source)) {
			return a2ui.DecodeBatch([]byte(source))
		}
		if m, ok := partialMessage(source, progressive); ok {
			return []a2ui.AgentMessage{m}, nil
		}
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(source))
	_, _ = decoder.Token()
	for decoder.More() {
		offset := int(decoder.InputOffset())
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) && err.Error() != "unexpected end of JSON input" {
				return messages, err
			}
			tail := strings.TrimLeft(source[offset:], ", \t\r\n")
			if m, ok := partialMessage(tail, progressive); ok {
				messages = append(messages, m)
			}
			break
		}
		m, err := a2ui.Decode(bytes.TrimSpace(raw))
		if err != nil {
			return messages, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// A mixed response may contain prose and several UI blocks. Commit each
// complete block independently so a malformed surface cannot leak into chat.
func (s *Stream) compileMixed(parts *[]Part) error {
	if len(s.block) > maxBlock {
		return fmt.Errorf("A2UI block exceeds %d bytes", maxBlock)
	}
	if s.State == nil {
		s.State = new(a2ui.Processor)
	}
	var messages []a2ui.AgentMessage
	var err error
	if s.isExpress() {
		messages, err = compileExpress(cleanExpress(s.block), s.Catalogs, s.State, true)
	} else {
		messages, err = decodeDirectJSON(cleanJSON(s.block))
	}
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return fmt.Errorf("empty A2UI block")
	}
	var updates, calls []a2ui.AgentMessage
	for _, message := range messages {
		if message.CallRenderer != nil || message.AgentResponse != nil {
			calls = append(calls, message)
		} else {
			updates = append(updates, message)
		}
	}
	if err := s.State.ApplyViewBatch(updates); err != nil {
		return err
	}
	for _, call := range calls {
		if s.ReceiveRPC != nil {
			if err := s.ReceiveRPC(call); err != nil {
				return err
			}
		}
	}
	*parts = append(*parts, Part{Messages: messages})
	return nil
}
