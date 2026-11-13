package a2uiagent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"
)

func (s *Stream) startExpress() {
	if s.State == nil {
		s.State = new(a2ui.Processor)
	}
	s.expressBase = s.State.Fork()
	s.expressComponents = make(map[string]string)
}

func (s *Stream) compileExpress(parts *[]Part, final bool) error {
	catalogs := s.Catalogs
	if len(catalogs) == 0 && s.Catalog != nil {
		catalogs = []*a2ui.Catalog{s.Catalog}
	}
	if len(catalogs) == 0 {
		return fmt.Errorf("Express stream requires a catalog")
	}
	if s.expressBase == nil {
		s.startExpress()
	}
	for _, catalog := range catalogs {
		if catalog == nil {
			return fmt.Errorf("missing Express catalog")
		}
		s.State.RegisterCatalog(catalog)
	}
	source := cleanExpress(s.block)
	if !final && source == "" {
		return nil
	}
	// Compile the whole candidate first. While receiving a statement, fall
	// back to the last line boundary that contains complete statements.
	messages, err := compileExpress(source, catalogs, s.expressBase, true)
	if err != nil && !final {
		if healed, ok := healExpress(source, s.ProgressiveFields); ok {
			messages, err = compileExpress(healed, catalogs, s.expressBase, true)
		}
	}
	if err != nil && !final {
		if strings.HasSuffix(source, "\n") && balancedExpress(source) && !strings.Contains(err.Error(), "unknown Express variable") {
			return err
		}
		prefix := source
		for err != nil {
			end := strings.LastIndexByte(prefix, '\n')
			if end < 0 {
				return nil
			}
			prefix = prefix[:end]
			messages, err = compileExpress(prefix, catalogs, s.expressBase, true)
		}
	}
	if err != nil {
		return err
	}
	var delta []a2ui.AgentMessage
	for _, message := range messages {
		raw, _ := json.Marshal(message)
		key := string(raw)
		if s.expressComponents[key] != "" {
			continue
		}
		if message.Components != nil && s.State.Surface(message.Components.ID) == nil {
			if final {
				return fmt.Errorf("unknown surface %q", message.Components.ID)
			}
			continue
		}
		if message.Data != nil && s.State.Surface(message.Data.ID) == nil {
			if final {
				return fmt.Errorf("unknown surface %q", message.Data.ID)
			}
			continue
		}
		if create := message.Create; create != nil && s.State.Surface(create.ID) != nil {
			var changed []a2ui.Component
			for _, component := range create.Components {
				raw, _ := json.Marshal(component)
				id := create.ID + "\x00" + component.ID
				if s.expressComponents[id] != string(raw) {
					changed = append(changed, component)
				}
			}
			if len(changed) > 0 {
				delta = append(delta, a2ui.AgentMessage{Version: a2ui.Version, Components: &a2ui.UpdateComponents{SurfaceRef: create.SurfaceRef, Components: changed}})
			}
			for _, path := range expressAssignments(source) {
				var next a2ui.Model
				_ = next.Set("", create.DataModel)
				value, _ := next.Get(path)
				raw, _ := json.Marshal(value)
				before, _ := s.State.Surface(create.ID).Data().Get(path)
				previous, _ := json.Marshal(before)
				if string(raw) != string(previous) {
					delta = append(delta, a2ui.AgentMessage{Version: a2ui.Version, Data: &a2ui.UpdateDataModel{SurfaceRef: create.SurfaceRef, Path: path, Value: raw}})
				}
			}
		} else {
			delta = append(delta, message)
		}
	}
	for _, message := range delta {
		if message.CallRenderer != nil || message.AgentResponse != nil {
			if message.CallRenderer != nil {
				if err := s.State.ValidateRendererCall(*message.CallRenderer); err != nil {
					return err
				}
			}
			if s.ReceiveRPC != nil {
				if err := s.ReceiveRPC(message); err != nil {
					return err
				}
			}
		} else if err := s.State.Apply(message); err != nil {
			return err
		}
	}
	for _, message := range messages {
		if message.Components != nil && s.State.Surface(message.Components.ID) == nil {
			continue
		}
		if message.Data != nil && s.State.Surface(message.Data.ID) == nil {
			continue
		}
		raw, _ := json.Marshal(message)
		s.expressComponents[string(raw)] = "emitted"
		if create := message.Create; create != nil {
			for _, component := range create.Components {
				raw, _ := json.Marshal(component)
				s.expressComponents[create.ID+"\x00"+component.ID] = string(raw)
			}
		}
	}
	if len(delta) > 0 {
		*parts = append(*parts, Part{Messages: delta})
	}
	if final {
		for _, message := range messages {
			id := ""
			if message.Create != nil {
				id = message.Create.ID
			}
			if message.Components != nil {
				id = message.Components.ID
			}
			if surface := s.State.Surface(id); surface != nil {
				if err := surface.ValidateReachable(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

var expressFence = regexp.MustCompile("(?i)^`{3}(?:a2ui|express)?[ \t]*\r?\n")
var expressText = regexp.MustCompile(`(?s)Text\s*\(\s*(?:text\s*=\s*)?"(?:[^"\\]|\\.)*$`)
var expressData = regexp.MustCompile(`(?s)\$([^\s=]+)\s*=\s*"(?:[^"\\]|\\.)*$`)

func cleanExpress(source string) string {
	source = strings.TrimLeft(source, " \t\r\n")
	source = expressFence.ReplaceAllString(source, "")
	return strings.TrimSuffix(strings.TrimRight(source, " \t\r\n"), "```") + source[len(strings.TrimRight(source, " \t\r\n")):]
}

func healExpress(source string, keys []string) (string, bool) {
	if slices.Contains(keys, "text") && expressText.MatchString(source) {
		return source + `")`, true
	}
	if match := expressData.FindStringSubmatch(source); match != nil {
		path := match[1]
		key := path[strings.LastIndexByte(path, '/')+1:]
		if slices.Contains(keys, key) {
			return source + `"`, true
		}
	}
	return "", false
}

func expressAssignments(source string) []string {
	statements, err := parseExpress(source)
	if err != nil {
		if healed, ok := healExpress(source, []string{"text"}); ok {
			statements, _ = parseExpress(healed)
		}
	}
	var paths []string
	for _, statement := range statements {
		if path, ok := strings.CutPrefix(statement.target, "$"); ok && !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	return paths
}
