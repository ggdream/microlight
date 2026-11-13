package a2ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Model stores the JSON values of a surface.
type Model struct {
	root   any
	Locale string
	// EvaluateFunction resolves a catalog function with evaluated arguments.
	EvaluateFunction func(FunctionCall, string) (any, error)
	defaultCatalog   *string
	observers        []*modelObserver
}

func pointer(path string) ([]string, error) {
	if path == "" || path == "/" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("expected absolute JSON pointer %q", path)
	}
	var parts []string
	for _, part := range strings.Split(path[1:], "/") {
		if part == "" {
			continue
		}
		var value strings.Builder
		for j := 0; j < len(part); j++ {
			if part[j] != '~' {
				value.WriteByte(part[j])
				continue
			}
			j++
			if j >= len(part) || (part[j] != '0' && part[j] != '1') {
				return nil, fmt.Errorf("invalid JSON pointer %q", path)
			}
			if part[j] == '0' {
				value.WriteByte('~')
			} else {
				value.WriteByte('/')
			}
		}
		segment := value.String()
		switch segment {
		case "__proto__", "constructor", "prototype":
			return nil, fmt.Errorf("forbidden path segment %q", segment)
		}
		parts = append(parts, segment)
	}
	return parts, nil
}

func arrayIndex(key string, size int) (int, bool) {
	index, err := strconv.Atoi(key)
	return index, err == nil && index >= 0 && index < size && strconv.Itoa(index) == key
}

func (m *Model) Get(path string) (any, bool) {
	value, found, _ := m.Lookup(path)
	return value, found
}

// Lookup is Get with a distinct error for malformed paths.
func (m *Model) Lookup(path string) (any, bool, error) {
	parts, err := pointer(path)
	if err != nil {
		return nil, false, err
	}
	value, found := getAt(m.root, parts)
	return value, found, nil
}

func getAt(root any, parts []string) (any, bool) {
	value := root
	if value == nil {
		value = map[string]any{}
	}
	for _, key := range parts {
		switch node := value.(type) {
		case map[string]any:
			var found bool
			value, found = node[key]
			if !found {
				return nil, false
			}
		case []any:
			index, ok := arrayIndex(key, len(node))
			if !ok {
				return nil, false
			}
			value = node[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func (m *Model) Set(path string, value any) error {
	parts, err := pointer(path)
	if err != nil {
		return err
	}
	if value == nil {
		if len(parts) == 0 {
			value = make(map[string]any)
		} else if _, exists := m.Get(path); !exists {
			return nil
		}
	}
	if err := validateDataDepth(value, len(parts)); err != nil {
		return err
	}
	root := m.root
	if root == nil && len(parts) != 0 {
		root = map[string]any{}
	}
	next, err := updateValue(root, parts, cloneJSON(value))
	if err == nil {
		previous := m.root
		m.root = next
		m.notify(previous)
	}
	return err
}

type modelObserver struct {
	path    []string
	changed func(any)
}

// Subscribe observes changes to the value at a JSON Pointer. The returned
// function releases the subscription. Subscribe and Set use the model owner's
// goroutine, like Processor and Surface.
func (m *Model) Subscribe(path string, changed func(any)) (func(), error) {
	parts, err := pointer(path)
	if err != nil {
		return nil, err
	}
	if changed == nil {
		return nil, fmt.Errorf("missing data observer")
	}
	observer := &modelObserver{path: parts, changed: changed}
	m.observers = append(m.observers, observer)
	return func() {
		observer.changed = nil
		m.observers = slices.DeleteFunc(m.observers, func(candidate *modelObserver) bool {
			return candidate == observer
		})
	}, nil
}

func (m *Model) disposeObservers() {
	for _, observer := range m.observers {
		observer.changed = nil
	}
	m.observers = nil
}

func (m *Model) notify(previous any) {
	if len(m.observers) == 0 {
		return
	}
	current := m.root
	for _, observer := range slices.Clone(m.observers) {
		before, beforeOK := getAt(previous, observer.path)
		after, afterOK := getAt(current, observer.path)
		if observer.changed != nil && (beforeOK != afterOK || !reflect.DeepEqual(before, after)) {
			observer.changed(cloneJSON(after))
		}
	}
}

func validateDataDepth(value any, depth int) error {
	if depth > 50 {
		return fmt.Errorf("A2UI data model exceeds 50 levels")
	}
	switch children := value.(type) {
	case map[string]any:
		for _, child := range children {
			if err := validateDataDepth(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range children {
			if err := validateDataDepth(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateValue(node any, parts []string, value any) (any, error) {
	if len(parts) == 0 {
		return value, nil
	}
	key, rest := parts[0], parts[1:]
	if node == nil {
		if index, err := strconv.Atoi(key); err == nil && index >= 0 && strconv.Itoa(index) == key {
			node = []any{}
		} else {
			node = make(map[string]any)
		}
	}
	switch current := node.(type) {
	case map[string]any:
		current = maps.Clone(current)
		if len(rest) == 0 && value == nil {
			delete(current, key)
			return current, nil
		}
		next, err := updateValue(current[key], rest, value)
		if err != nil {
			return nil, err
		}
		current[key] = next
		return current, nil
	case []any:
		index, ok := arrayIndex(key, 10001)
		if !ok {
			return nil, fmt.Errorf("invalid array index %q", key)
		}
		current = slices.Clone(current)
		if index >= len(current) {
			current = append(current, make([]any, index+1-len(current))...)
		}
		next, err := updateValue(current[index], rest, value)
		if err != nil {
			return nil, err
		}
		current[index] = next
		return current, nil
	default:
		return nil, fmt.Errorf("cannot write through scalar at %q", key)
	}
}

func decodeValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	err := decoder.Decode(&value)
	return value, err
}

// Resolve evaluates literal JSON values and bindings relative to scope.
func (m *Model) Resolve(value any, scope string) (any, error) {
	switch node := value.(type) {
	case map[string]any:
		if binding, ok := node["@path"]; ok {
			path, ok := binding.(string)
			if !ok {
				return nil, fmt.Errorf("@path must be a string")
			}
			resolved, ok, err := m.Lookup(ResolvePath(scope, path))
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, nil
			}
			return resolved, nil
		}
		if raw, ok := node["@call"]; ok {
			name, ok := raw.(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("@call must be a function name")
			}
			call := FunctionCall{Name: name}
			call.Catalog, _ = node["catalogId"].(string)
			if args, ok := node["args"]; ok {
				resolved, err := m.Resolve(args, scope)
				if err != nil {
					return nil, err
				}
				call.Args, ok = resolved.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("function arguments must be an object")
				}
			}
			return m.evaluate(call, scope)
		}
		result := make(map[string]any, len(node))
		for key, item := range node {
			resolved, err := m.Resolve(item, scope)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(key, "@@") {
				key = key[1:]
			}
			result[key] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, len(node))
		for i, item := range node {
			resolved, err := m.Resolve(item, scope)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil
	default:
		return value, nil
	}
}

func ResolvePath(scope, path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	if path == "" || path == "." {
		if scope != "/" {
			scope = strings.TrimSuffix(scope, "/")
		}
		return scope
	}
	return strings.TrimSuffix(scope, "/") + "/" + path
}

func BindingPath(raw json.RawMessage, scope string) (string, bool) {
	var binding struct {
		Path *string `json:"@path"`
	}
	if json.Unmarshal(raw, &binding) != nil || binding.Path == nil {
		return "", false
	}
	return ResolvePath(scope, *binding.Path), true
}

func (m *Model) evaluate(call FunctionCall, scope string) (any, error) {
	if call.Catalog == "" && call.Name != "@index" && m.defaultCatalog != nil {
		call.Catalog = *m.defaultCatalog
		if call.Catalog == "" {
			return nil, fmt.Errorf("function %q has no default catalogId", call.Name)
		}
	}
	if m.EvaluateFunction != nil {
		return m.EvaluateFunction(call, scope)
	}
	return m.EvaluateLocal(call, scope)
}

// EvaluateLocal bypasses a host dispatcher and evaluates a built-in function.
func (m *Model) EvaluateLocal(call FunctionCall, scope string) (any, error) {
	if call.Catalog != "" && call.Catalog != BasicCatalog && call.Catalog != "basic" {
		return nil, fmt.Errorf("no implementation for catalog %q", call.Catalog)
	}
	return m.basicFunction(call, scope)
}
