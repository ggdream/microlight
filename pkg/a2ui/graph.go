package a2ui

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

func componentChildren(c Component) []string {
	var children []string
	for _, field := range []string{"child", "trigger", "content"} {
		var id string
		if json.Unmarshal(c.Props[field], &id) == nil && id != "" {
			children = append(children, id)
		}
	}
	var ids []string
	if json.Unmarshal(c.Props["children"], &ids) == nil {
		children = append(children, ids...)
	} else {
		var template struct{ ComponentID string }
		if json.Unmarshal(c.Props["children"], &template) == nil && template.ComponentID != "" {
			children = append(children, template.ComponentID)
		}
	}
	for _, field := range []string{"tabs", "tabItems"} {
		var tabs []struct{ Child string }
		if json.Unmarshal(c.Props[field], &tabs) == nil {
			for _, tab := range tabs {
				if tab.Child != "" {
					children = append(children, tab.Child)
				}
			}
		}
	}
	return children
}

func validateGraph(components map[string]Component, children map[string][]string, complete, requireReachable bool) error {
	states := make(map[string]uint8, len(components))
	heights := make(map[string]int, len(components))
	var visit func(string, int) error
	visit = func(id string, depth int) error {
		if depth > 50 {
			return fmt.Errorf("A2UI component graph exceeds 50 levels")
		}
		switch states[id] {
		case 1:
			return fmt.Errorf("cycle at A2UI component %q", id)
		case 2:
			if depth+heights[id] > 50 {
				return fmt.Errorf("A2UI component graph exceeds 50 levels")
			}
			return nil
		}
		_, found := components[id]
		if !found {
			if complete {
				return fmt.Errorf("missing A2UI component %q", id)
			}
			return nil
		}
		states[id] = 1
		for _, child := range children[id] {
			if err := visit(child, depth+1); err != nil {
				return err
			}
			heights[id] = max(heights[id], heights[child]+1)
		}
		states[id] = 2
		return nil
	}
	if len(components) == 0 {
		return nil
	}
	if err := visit(Root, 0); err != nil {
		return err
	}
	for id := range components {
		if requireReachable && states[id] == 0 {
			return fmt.Errorf("unreachable A2UI component %q", id)
		}
		if err := visit(id, 0); err != nil {
			return err
		}
	}
	return nil
}

// ValidateReachable checks the visible graph while permitting cached, detached components.
func (s *Surface) ValidateReachable() error {
	reachable := make(map[string]Component)
	var visit func(string)
	visit = func(id string) {
		if _, seen := reachable[id]; seen {
			return
		}
		if node, ok := s.components[id]; ok {
			reachable[id] = node
			for _, child := range s.children[id] {
				visit(child)
			}
		}
	}
	visit(Root)
	if len(reachable) == 0 && len(s.components) > 0 {
		return fmt.Errorf("missing A2UI root")
	}
	return validateGraph(reachable, s.children, true, false)
}

func (s *Surface) ValidateGraph() error { return s.ValidateTopology(false, false) }

// ValidateTopology checks the current component graph with the requested
// allowances for staged component updates.
func (s *Surface) ValidateTopology(allowOrphans, allowDangling bool) error {
	return validateGraph(s.components, s.children, !allowDangling, !allowOrphans)
}

func (p *Processor) update(s *Surface, components []Component) error {
	seen := make(map[string]bool, len(components))
	candidate := maps.Clone(s.components)
	children := make(map[string][]string, len(candidate)+len(components))
	maps.Copy(children, s.children)
	for _, component := range components {
		if seen[component.ID] {
			return fmt.Errorf("duplicate component %q", component.ID)
		}
		seen[component.ID] = true
		id := component.Catalog
		if id == "" {
			id = s.catalog
		}
		catalog, err := p.catalog(id)
		if err != nil {
			return err
		}
		if raw, ok := component.Props["catalogId"]; ok && component.Catalog == "" && !catalog.declaresProperty(component.Type, "catalogId") {
			var legacyID string
			if err := json.Unmarshal(raw, &legacyID); err != nil || legacyID == "" {
				return fmt.Errorf("invalid catalogId for component %q", component.ID)
			}
			catalog, err = p.catalog(legacyID)
			if err != nil {
				return err
			}
			component.Catalog = legacyID
			component.Props = maps.Clone(component.Props)
			delete(component.Props, "catalogId")
		}
		if component.Catalog == "" && s.catalog == "" && len(p.catalogs) != 0 {
			return fmt.Errorf("component %q has no default catalogId", component.ID)
		}
		for _, property := range []string{"accessibility", "checks", "metadata"} {
			if raw, ok := component.Props[property]; ok && !catalog.declaresProperty(component.Type, property) {
				prefixed := "@" + property
				if component.Props[prefixed] != nil {
					return fmt.Errorf("component %q declares both %s and %s", component.ID, property, prefixed)
				}
				component.Props = maps.Clone(component.Props)
				component.Props[prefixed] = raw
				delete(component.Props, property)
			}
		}
		if _, known := catalog.Components[component.Type]; known || !p.allowViewFallbacks {
			if err := catalog.ValidateComponent(component); err != nil {
				return fmt.Errorf("component %q: %w", component.ID, err)
			}
		}
		for _, raw := range component.Props {
			value, err := decodeValue(raw)
			if err == nil {
				if err := p.validateCalls(value, s.catalog, 0); err != nil {
					return fmt.Errorf("component %q: %w", component.ID, err)
				}
			}
		}
		candidate[component.ID] = component
		children[component.ID] = catalog.childReferences(component)
	}
	if !p.allowViewFallbacks {
		if err := validateGraph(candidate, children, false, false); err != nil {
			return err
		}
	}
	if err := p.validateComposition(s.catalog, candidate, children); err != nil {
		return err
	}
	s.components, s.children = candidate, children
	return nil
}

func (p *Processor) validateCalls(value any, defaultCatalog string, depth int) error {
	switch value := value.(type) {
	case []any:
		for _, item := range value {
			if err := p.validateCalls(item, defaultCatalog, depth); err != nil {
				return err
			}
		}
	case map[string]any:
		if path, ok := value["@path"].(string); ok {
			if _, err := pointer(ResolvePath("", path)); err != nil {
				return err
			}
		}
		name, isCall := value["@call"].(string)
		if isCall {
			depth++
			if depth > 5 {
				return fmt.Errorf("A2UI functionCall depth exceeds 5")
			}
		}
		if isCall && name != "@index" {
			id, _ := value["catalogId"].(string)
			if id == "" {
				if _, explicit := value["catalogId"]; explicit {
					return fmt.Errorf("function %q has an invalid catalogId", name)
				}
				id = defaultCatalog
				if id == "" && len(p.catalogs) != 0 {
					return fmt.Errorf("function %q has no default catalogId", name)
				}
			}
			catalog, err := p.catalog(id)
			if err != nil {
				return err
			}
			if !catalog.RendererMayCall(name) {
				return fmt.Errorf("function %q is agentOnly", name)
			}
			args, _ := value["args"].(map[string]any)
			override, _ := value["catalogId"].(string)
			if _, declared := catalog.Functions[name]; declared || !p.allowUndeclaredFunctions {
				if err := catalog.ValidateFunction(FunctionCall{Name: name, Catalog: override, Args: args}); err != nil {
					return err
				}
			}
		}
		for _, item := range value {
			if err := p.validateCalls(item, defaultCatalog, depth); err != nil {
				return err
			}
		}
	}
	return nil
}

type compositionRules struct {
	AllowedParents  []string `json:"allowedParents"`
	AllowedChildren []string `json:"allowedChildren"`
}

func (p *Processor) validateComposition(defaultCatalog string, components map[string]Component, children map[string][]string) error {
	rules := make(map[string]compositionRules, len(components))
	for id, component := range components {
		catalog := component.Catalog
		if catalog == "" {
			catalog = defaultCatalog
		}
		c, err := p.catalog(catalog)
		if err != nil {
			return err
		}
		var rule compositionRules
		_ = json.Unmarshal(c.Components[component.Type], &rule)
		rules[id] = rule
	}
	if root, exists := components[Root]; exists && rules[Root].AllowedParents != nil && !slices.Contains(rules[Root].AllowedParents, "Surface") {
		return fmt.Errorf("UNALLOWED_PARENT: Surface cannot contain %q", root.Type)
	}
	for id, parent := range components {
		for _, childID := range children[id] {
			child, exists := components[childID]
			if !exists {
				continue
			}
			if allowed := rules[id].AllowedChildren; allowed != nil && !slices.Contains(allowed, child.Type) {
				return fmt.Errorf("UNALLOWED_CHILD: %q cannot contain %q", parent.Type, child.Type)
			}
			if allowed := rules[childID].AllowedParents; allowed != nil && !slices.Contains(allowed, parent.Type) {
				return fmt.Errorf("UNALLOWED_PARENT: %q cannot contain %q", parent.Type, child.Type)
			}
		}
	}
	return nil
}

// childReferences follows the catalog schema for custom child properties,
// while retaining the Basic Catalog's structured children and tab handling.
func (c *Catalog) childReferences(component Component) []string {
	children := componentChildren(component)
	var schema childSchema
	if json.Unmarshal(c.Components[component.Type], &schema) != nil {
		return children
	}
	for name, field := range schema.Properties {
		if slices.Contains([]string{"child", "children", "trigger", "content", "tabs", "tabItems"}, name) {
			continue
		}
		value, err := decodeValue(component.Props[name])
		if err == nil {
			children = c.collectReferences(children, field, value, 0)
		}
	}
	return children
}

type childSchema struct {
	Ref        string                 `json:"$ref"`
	Items      *childSchema           `json:"items"`
	Properties map[string]childSchema `json:"properties"`
}

func (c *Catalog) collectReferences(ids []string, schema childSchema, value any, depth int) []string {
	if depth > 50 {
		return ids
	}
	_, name, isRef := strings.Cut(schema.Ref, "#/$defs/")
	if isRef {
		switch name {
		case "ComponentId":
			if id, ok := value.(string); ok {
				return append(ids, id)
			}
		default:
			var target childSchema
			if json.Unmarshal(c.Definitions[name], &target) == nil {
				return c.collectReferences(ids, target, value, depth+1)
			}
		}
	}
	switch value := value.(type) {
	case []any:
		if schema.Items != nil {
			for _, item := range value {
				ids = c.collectReferences(ids, *schema.Items, item, depth+1)
			}
		}
	case map[string]any:
		for key, prop := range schema.Properties {
			ids = c.collectReferences(ids, prop, value[key], depth+1)
		}
	}
	return ids
}

// declaresProperty distinguishes a custom catalog property from the legacy
// unprefixed component routing field.
func (c *Catalog) declaresProperty(kind, property string) bool {
	return customProperty(c.Components[kind], property)
}

func customProperty(raw json.RawMessage, property string) bool {
	var schema struct {
		Properties map[string]json.RawMessage
		AllOf      []json.RawMessage
		AnyOf      []json.RawMessage
		OneOf      []json.RawMessage
	}
	if json.Unmarshal(raw, &schema) != nil {
		return false
	}
	if field := schema.Properties[property]; field != nil {
		if schema.Properties["id"] == nil {
			return true
		}
		var shape struct {
			Type       string
			Properties map[string]json.RawMessage
		}
		_ = json.Unmarshal(field, &shape)
		switch property {
		case "catalogId":
			return shape.Type != "string"
		case "metadata":
			return shape.Type != "object" || len(shape.Properties) != 1 || shape.Properties["extensions"] == nil
		default:
			return true
		}
	}
	for _, branches := range [][]json.RawMessage{schema.AllOf, schema.AnyOf, schema.OneOf} {
		for _, branch := range branches {
			if customProperty(branch, property) {
				return true
			}
		}
	}
	return false
}
