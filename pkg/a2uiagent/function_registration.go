package a2uiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"
)

type FunctionRegistration struct {
	Name                   string
	FromCatalog            bool
	ReturnType             string
	Description            string
	RequiresUserActivation bool
	Arguments              json.RawMessage
	Handler                AgentFunction
}

// RegisterFunctions produces the agent's copy of a catalog and its handlers.
func RegisterFunctions(base *a2ui.Catalog, registrations []FunctionRegistration) (*a2ui.Catalog, map[string]AgentFunction, error) {
	if base == nil {
		return nil, nil, fmt.Errorf("missing agent catalog")
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(base.Document(), &doc); err != nil {
		return nil, nil, err
	}
	functions := make(map[string]json.RawMessage)
	defs := make(map[string]json.RawMessage)
	_ = json.Unmarshal(doc["functions"], &functions)
	_ = json.Unmarshal(doc["$defs"], &defs)
	handlers := make(map[string]AgentFunction, len(registrations))
	for _, entry := range registrations {
		if !validIdentifier(entry.Name) || entry.Handler == nil || handlers[entry.Name] != nil {
			return nil, nil, fmt.Errorf("invalid or duplicate agent function %q", entry.Name)
		}
		args := entry.Arguments
		if entry.FromCatalog {
			var definition struct {
				Callers    string `json:"allowedCallers"`
				Activation bool   `json:"requiresUserActivation"`
			}
			if err := json.Unmarshal(functions[entry.Name], &definition); err != nil ||
				definition.Callers == "agentOnly" || definition.Activation && definition.Callers == "rendererOrAgent" {
				return nil, nil, fmt.Errorf("function %q cannot handle renderer calls", entry.Name)
			}
			args = functionArgs(functions[entry.Name])
		} else {
			if functions[entry.Name] != nil {
				return nil, nil, fmt.Errorf("function %q already exists", entry.Name)
			}
			if len(args) == 0 {
				args = json.RawMessage(`{"type":"object"}`)
			}
			var schema map[string]json.RawMessage
			if err := json.Unmarshal(args, &schema); err != nil {
				return nil, nil, err
			}
			var local map[string]json.RawMessage
			_ = json.Unmarshal(schema["$defs"], &local)
			for name, value := range local {
				if existing := defs[name]; existing != nil && !sameSchema(existing, value) {
					return nil, nil, fmt.Errorf("conflicting catalog definition %q", name)
				}
				defs[name] = value
			}
			raw, err := json.Marshal(map[string]any{
				"type": "object", "description": entry.Description, "returnType": entry.ReturnType,
				"requiresUserActivation": entry.RequiresUserActivation,
				"properties":             map[string]any{"@call": map[string]string{"const": entry.Name}, "args": args},
				"required":               []string{"@call", "args"},
			})
			if err != nil {
				return nil, nil, err
			}
			functions[entry.Name] = raw
		}
		if err := validateArgumentNames(args, defs, make(map[string]bool)); err != nil {
			return nil, nil, fmt.Errorf("function %s: %w", entry.Name, err)
		}
		handlers[entry.Name] = entry.Handler
	}
	doc["functions"], _ = json.Marshal(functions)
	doc["$defs"], _ = json.Marshal(defs)
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, err
	}
	catalog, err := a2ui.ParseCatalog(raw)
	return catalog, handlers, err
}

func sameSchema(first, second json.RawMessage) bool {
	normalize := func(raw json.RawMessage) []byte {
		var value any
		_ = json.Unmarshal(raw, &value)
		out, _ := json.Marshal(value)
		return out
	}
	return bytes.Equal(normalize(first), normalize(second))
}

func validateArgumentNames(raw json.RawMessage, defs map[string]json.RawMessage, seen map[string]bool) error {
	var schema struct {
		Properties          map[string]json.RawMessage
		Ref                 string `json:"$ref"`
		AllOf, AnyOf, OneOf []json.RawMessage
	}
	if len(raw) == 0 || json.Unmarshal(raw, &schema) != nil {
		return nil
	}
	for name := range schema.Properties {
		if !validIdentifier(name) {
			return fmt.Errorf("invalid parameter %q", name)
		}
	}
	for _, alternatives := range [][]json.RawMessage{schema.AllOf, schema.AnyOf, schema.OneOf} {
		for _, alternative := range alternatives {
			if err := validateArgumentNames(alternative, defs, seen); err != nil {
				return err
			}
		}
	}
	if ref, ok := strings.CutPrefix(schema.Ref, "#/$defs/"); ok && !seen[ref] {
		seen[ref] = true
		segments := strings.Split(ref, "/")
		target := defs[segments[0]]
		for _, segment := range segments[1:] {
			var value map[string]json.RawMessage
			if json.Unmarshal(target, &value) == nil {
				target = value[segment]
				continue
			}
			var items []json.RawMessage
			index, err := strconv.Atoi(segment)
			if err != nil || json.Unmarshal(target, &items) != nil || index < 0 || index >= len(items) {
				return fmt.Errorf("invalid definition reference %q", ref)
			}
			target = items[index]
		}
		if target == nil {
			return fmt.Errorf("unknown definition reference %q", ref)
		}
		return validateArgumentNames(target, defs, seen)
	}
	return nil
}
