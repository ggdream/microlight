package a2ui

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
)

func applyFunctionDefaults(args map[string]any, raw json.RawMessage, defs map[string]json.RawMessage, depth int) error {
	if len(raw) == 0 || depth > 32 || raw[0] != '{' {
		return nil
	}
	var schema struct {
		Ref        string                     `json:"$ref"`
		Defs       map[string]json.RawMessage `json:"$defs"`
		AllOf      []json.RawMessage
		Properties map[string]json.RawMessage
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return err
	}
	if len(schema.Defs) != 0 {
		merged := make(map[string]json.RawMessage, len(defs)+len(schema.Defs))
		maps.Copy(merged, defs)
		maps.Copy(merged, schema.Defs)
		defs = merged
	}
	if ref, ok := strings.CutPrefix(schema.Ref, "#/$defs/"); ok {
		if err := applyFunctionDefaults(args, defs[ref], defs, depth+1); err != nil {
			return err
		}
	}
	for _, child := range schema.AllOf {
		if err := applyFunctionDefaults(args, child, defs, depth+1); err != nil {
			return err
		}
	}
	for name, raw := range schema.Properties {
		if current, exists := args[name]; exists {
			if nested, ok := current.(map[string]any); ok {
				if err := applyFunctionDefaults(nested, raw, defs, depth+1); err != nil {
					return err
				}
			}
			continue
		}
		var field struct{ Default json.RawMessage }
		if json.Unmarshal(raw, &field) != nil || len(field.Default) == 0 {
			continue
		}
		value, err := decodeValue(field.Default)
		if err != nil {
			return fmt.Errorf("default argument %q: %w", name, err)
		}
		args[name] = value
	}
	return nil
}
