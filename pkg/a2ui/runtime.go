package a2ui

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

type Capabilities struct {
	CatalogIDs     []string          `json:"supportedCatalogIds"`
	InlineCatalogs []json.RawMessage `json:"inlineCatalogs,omitempty"`
}

type RendererCapabilities map[string]Capabilities

type AgentCapabilities map[string]AgentVersionCapabilities

type AgentVersionCapabilities struct {
	CatalogIDs            []string `json:"supportedCatalogIds"`
	AcceptsInlineCatalogs bool     `json:"acceptsInlineCatalogs,omitempty"`
}

type RendererDataModel struct {
	Version  string         `json:"version"`
	Surfaces map[string]any `json:"surfaces"`
}

func (p *Processor) RegisterCatalog(catalog *Catalog) {
	if p.catalogs == nil {
		p.catalogs = make(map[string]*Catalog)
	}
	p.catalogs[catalog.ID] = catalog
}

func (p *Processor) Capabilities(inline bool) RendererCapabilities {
	ids := slices.Collect(maps.Keys(p.catalogs))
	if len(p.catalogs) == 0 {
		ids = append(ids, BasicCatalog)
	}
	sort.Strings(ids)
	capabilities := Capabilities{CatalogIDs: ids}
	if inline {
		for _, id := range ids {
			if c := p.catalogs[id]; c != nil {
				capabilities.InlineCatalogs = append(capabilities.InlineCatalogs, c.Document())
			}
		}
	}
	return RendererCapabilities{Version: capabilities}
}

func (p *Processor) DataModel() RendererDataModel {
	snapshot := RendererDataModel{Version: Version, Surfaces: make(map[string]any)}
	for id, surface := range p.surfaces {
		if surface.sendDataModel {
			snapshot.Surfaces[id] = cloneJSON(surface.data.root)
		}
	}
	return snapshot
}

func cloneJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(v))
		for key, item := range v {
			copy[key] = cloneJSON(item)
		}
		return copy
	case []any:
		copy := make([]any, len(v))
		for i, item := range v {
			copy[i] = cloneJSON(item)
		}
		return copy
	default:
		return v
	}
}

func (p *Processor) catalog(id string) (*Catalog, error) {
	if catalog := p.catalogs[id]; catalog != nil {
		return catalog, nil
	}
	if id == "" || id == BasicCatalog {
		return basicCatalog()
	}
	return nil, fmt.Errorf("unknown A2UI catalog %q", id)
}

// ApplyBatch applies a list atomically. Each message still observes preceding messages.
func (p *Processor) ApplyBatch(messages []AgentMessage) error {
	return p.applyBatch(messages, false)
}

// ApplyViewBatch also verifies the reachable component graphs before commit.
func (p *Processor) ApplyViewBatch(messages []AgentMessage) error {
	return p.applyBatch(messages, true)
}

func (p *Processor) applyBatch(messages []AgentMessage, validateView bool) error {
	if len(messages) == 0 {
		return nil
	}
	candidate := p.Fork()
	touched := make(map[string]bool)
	replaced := make(map[string]bool)
	for i, message := range messages {
		if message.CallRenderer != nil || message.AgentResponse != nil {
			return fmt.Errorf("message %d: dispatch RPC separately from state batches", i)
		}
		if err := candidate.Apply(message); err != nil {
			return fmt.Errorf("message %d: %w", i, err)
		}
		if message.Delete != nil {
			replaced[message.Delete.ID] = true
		}
		if validateView {
			if message.Create != nil {
				touched[message.Create.ID] = true
			}
			if message.Components != nil {
				touched[message.Components.ID] = true
			}
		}
	}
	for id := range touched {
		if surface := candidate.Surface(id); surface != nil {
			if err := surface.ValidateReachable(); err != nil {
				return err
			}
		}
	}
	for id, live := range p.surfaces {
		if replaced[id] || candidate.Surface(id) == nil {
			live.data.disposeObservers()
		}
	}
	previous := make(map[*Model]any)
	for id, surface := range candidate.surfaces {
		if live := p.Surface(id); live != nil && !replaced[id] {
			surface.data.observers = live.data.observers
			previous[&live.data] = live.data.root
			*live = *surface
			candidate.surfaces[id] = live
		}
	}
	p.surfaces = candidate.surfaces
	for model, root := range previous {
		model.notify(root)
	}
	return nil
}

func DecodeBatch(data []byte) ([]AgentMessage, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		var wrapper struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil || wrapper.Messages == nil {
			message, err := Decode(data)
			if err != nil {
				return nil, err
			}
			return []AgentMessage{message}, nil
		}
		raw = wrapper.Messages
	}
	messages := make([]AgentMessage, 0, len(raw))
	for i, entry := range raw {
		message, err := Decode(entry)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", i, err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (s *Surface) Checks(component Component, scope string) []ValidationResult {
	var checks []struct {
		Condition json.RawMessage `json:"condition"`
		Message   string          `json:"message"`
	}
	_ = json.Unmarshal(component.Props["@checks"], &checks)
	var failures []ValidationResult
	for _, check := range checks {
		result, err := s.Resolve(check.Condition, scope)
		if err == nil && Boolean(result) {
			continue
		}
		var failure ValidationResult
		switch result := result.(type) {
		case ValidationResult:
			failure = result
		case map[string]any:
			failure.Message, _ = result["message"].(string)
			failure.Code, _ = result["code"].(string)
			failure.Severity, _ = result["severity"].(string)
		}
		if check.Message != "" {
			failure.Message = check.Message
		}
		if failure.Message == "" {
			failure.Message = "Validation failed"
		}
		if failure.Severity != "warning" && failure.Severity != "info" {
			failure.Severity = "error"
		}
		failure.Valid = false
		failures = append(failures, failure)
	}
	return failures
}

// IsValid reports whether any check prevents the component from being used.
func (s *Surface) IsValid(component Component, scope string) bool {
	for _, result := range s.Checks(component, scope) {
		if result.Severity == "error" {
			return false
		}
	}
	return true
}

func (s *Surface) Activate(componentID, scope string, now time.Time, open func(string) error) (*RendererMessage, error) {
	return s.Invoke(componentID, scope, now, true, open)
}

// Invoke evaluates a component action with an explicit activation context.
func (s *Surface) Invoke(componentID, scope string, now time.Time, userActivation bool, open func(string) error) (*RendererMessage, error) {
	component, ok := s.Component(componentID)
	if !ok {
		return nil, fmt.Errorf("unknown action source %q", componentID)
	}
	var action struct {
		Function json.RawMessage `json:"functionCall"`
	}
	if err := json.Unmarshal(component.Props["action"], &action); err != nil {
		return nil, err
	}
	if len(action.Function) == 0 {
		event, err := s.Action(componentID, scope, now)
		return &event, err
	}
	var call FunctionCall
	if err := json.Unmarshal(action.Function, &call); err != nil {
		return nil, err
	}
	if !userActivation && s.requiresActivation(call) {
		return nil, fmt.Errorf("%s requires user activation", call.Name)
	}
	if call.Name != "openUrl" || (call.Catalog != "" && call.Catalog != BasicCatalog && call.Catalog != "basic") {
		_, err := s.Resolve(action.Function, scope)
		return nil, err
	}
	value, err := s.data.Resolve(call.Args["url"], scope)
	if err != nil {
		return nil, err
	}
	u, err := SafeURL(Text(value))
	if err != nil {
		return nil, err
	}
	if open == nil {
		return nil, fmt.Errorf("no host URL opener")
	}
	return nil, open(u.String())
}

// Fork returns an independent snapshot for speculative processing.
func (p *Processor) Fork() *Processor {
	candidate := *p
	candidate.catalogs = maps.Clone(p.catalogs)
	candidate.surfaces = make(map[string]*Surface, len(p.surfaces))
	for id, surface := range p.surfaces {
		copy := *surface
		copy.components = maps.Clone(surface.components)
		copy.data.root = cloneJSON(surface.data.root)
		copy.data.observers = nil
		candidate.surfaces[id] = &copy
	}
	return &candidate
}

// ValidateRendererCall checks that an agent may call the named renderer function.
func (p *Processor) ValidateRendererCall(request CallRendererFunction) error {
	if request.Surface != "" && p.Surface(request.Surface) == nil {
		return fmt.Errorf("unknown A2UI surface %q", request.Surface)
	}
	catalog, err := p.catalog(request.Function.Catalog)
	if err != nil {
		return err
	}
	if !catalog.AgentMayCall(request.Function.Name) {
		return fmt.Errorf("agent may not call %q", request.Function.Name)
	}
	if err := catalog.ValidateFunction(request.Function); err != nil {
		return err
	}
	defaultID := catalog.ID
	if surface := p.Surface(request.Surface); surface != nil {
		defaultID = surface.Catalog()
	}
	return p.validateRendererArguments(request.Function.Args, defaultID, 1)
}

func (p *Processor) validateRendererArguments(value any, catalogID string, depth int) error {
	switch value := value.(type) {
	case []any:
		for _, item := range value {
			if err := p.validateRendererArguments(item, catalogID, depth); err != nil {
				return err
			}
		}
	case map[string]any:
		if path, present := value["@path"]; present {
			text, ok := path.(string)
			if !ok || len(value) != 1 {
				return fmt.Errorf("invalid renderer function binding")
			}
			_, err := pointer(ResolvePath("", text))
			return err
		}
		if raw, present := value["@call"]; present {
			name, ok := raw.(string)
			if !ok || name == "" || name == "@index" || depth >= 5 {
				return fmt.Errorf("invalid nested renderer function")
			}
			call := FunctionCall{Name: name}
			for key, item := range value {
				switch key {
				case "@call":
				case "catalogId":
					call.Catalog, ok = item.(string)
					if !ok || call.Catalog == "" {
						return fmt.Errorf("invalid nested renderer function catalog")
					}
					catalogID = call.Catalog
				case "args":
					call.Args, ok = item.(map[string]any)
					if !ok {
						return fmt.Errorf("invalid nested renderer function arguments")
					}
				default:
					return fmt.Errorf("unexpected function field %q", key)
				}
			}
			if catalogID == "" {
				return fmt.Errorf("function %q needs a default catalog or catalogId", name)
			}
			catalog, err := p.catalog(catalogID)
			if err != nil {
				return err
			}
			if !catalog.RendererMayCall(name) {
				return fmt.Errorf("function %q is agentOnly", name)
			}
			if catalog.requiresActivation(name) {
				return fmt.Errorf("function %q requires user activation", name)
			}
			if err := catalog.ValidateFunction(call); err != nil {
				return err
			}
			return p.validateRendererArguments(call.Args, catalogID, depth+1)
		}
		for key, item := range value {
			if strings.HasPrefix(key, "@") && !strings.HasPrefix(key, "@@") {
				return fmt.Errorf("unknown renderer function directive %q", key)
			}
			if err := p.validateRendererArguments(item, catalogID, depth); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Surface) requiresActivation(call FunctionCall) bool {
	id := call.Catalog
	if id == "" {
		id = s.Catalog()
	}
	if (id == "" || id == "basic" || id == BasicCatalog) && call.Name == "openUrl" {
		return true
	}
	if catalog := s.catalogs[id]; catalog != nil {
		return catalog.requiresActivation(call.Name)
	}
	return false
}
