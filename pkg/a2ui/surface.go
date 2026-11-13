package a2ui

import (
	"encoding/json"
	"fmt"
	"time"
)

// Processor applies messages in arrival order. Call it from the owner of the UI state.
type Processor struct {
	surfaces map[string]*Surface
	catalogs map[string]*Catalog
}

type Surface struct {
	id            string
	catalog       string
	components    map[string]Component
	children      map[string][]string
	data          Model
	sendDataModel bool
	metadata      *Metadata
}

func (p *Processor) Surface(id string) *Surface { return p.surfaces[id] }
func (s *Surface) ID() string                   { return s.id }
func (s *Surface) Catalog() string              { return s.catalog }
func (s *Surface) Data() *Model                 { return &s.data }
func (s *Surface) SendDataModel() bool          { return s.sendDataModel }
func (s *Surface) Metadata() *Metadata          { return s.metadata }
func (s *Surface) Component(id string) (Component, bool) {
	component, ok := s.components[id]
	return component, ok
}

// Apply handles surface lifecycle and state messages. Function messages are
// handled by the application transport, which owns function registration.
func (p *Processor) Apply(m AgentMessage) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := ValidateProtocol("agent_to_renderer.json", data); err != nil {
		return err
	}
	switch {
	case m.Create != nil:
		c := m.Create
		if _, err := p.catalog(c.Catalog); err != nil {
			return err
		}
		if p.Surface(c.ID) != nil {
			return fmt.Errorf("surface %q already exists", c.ID)
		}
		if err := validateDataDepth(c.DataModel, 0); err != nil {
			return err
		}
		data, _ := cloneJSON(c.DataModel).(map[string]any)
		if data == nil {
			data = make(map[string]any)
		}
		s := &Surface{id: c.ID, catalog: c.Catalog, components: make(map[string]Component),
			data: Model{root: data}, sendDataModel: c.SendDataModel, metadata: c.Metadata}
		if c.Catalog != "" || len(p.catalogs) > 0 {
			s.data.defaultCatalog = new(c.Catalog)
		}
		if err := p.update(s, c.Components); err != nil {
			return err
		}
		if p.surfaces == nil {
			p.surfaces = make(map[string]*Surface)
		}
		p.surfaces[c.ID] = s
	case m.Components != nil:
		s := p.Surface(m.Components.ID)
		if s == nil {
			return fmt.Errorf("unknown surface %q", m.Components.ID)
		}
		return p.update(s, m.Components.Components)
	case m.Data != nil:
		s := p.Surface(m.Data.ID)
		if s == nil {
			return fmt.Errorf("unknown surface %q", m.Data.ID)
		}
		value, err := decodeValue(m.Data.Value)
		if err != nil {
			return err
		}
		return s.data.Set(m.Data.Path, value)
	case m.Delete != nil:
		if surface := p.Surface(m.Delete.ID); surface != nil {
			surface.data.disposeObservers()
		}
		delete(p.surfaces, m.Delete.ID)
	default:
		return fmt.Errorf("function message requires a host handler")
	}
	return nil
}

func (s *Surface) Resolve(raw json.RawMessage, scope string) (any, error) {
	value, err := decodeValue(raw)
	if err != nil {
		return nil, err
	}
	return s.data.Resolve(value, scope)
}

func (s *Surface) String(raw json.RawMessage, scope string) string {
	value, _ := s.Resolve(raw, scope)
	text, _ := value.(string)
	return text
}

// Action resolves the event declared by a component against the current data model.
func (s *Surface) Action(componentID, scope string, now time.Time) (RendererMessage, error) {
	return s.ActionWithMetadata(componentID, scope, now, nil)
}

// ActionWithMetadata attaches dispatch metadata supplied by the renderer host.
func (s *Surface) ActionWithMetadata(componentID, scope string, now time.Time, metadata *Metadata) (RendererMessage, error) {
	component, ok := s.Component(componentID)
	if !ok {
		return RendererMessage{}, fmt.Errorf("unknown action source %q", componentID)
	}
	var action struct {
		Event *Event `json:"event"`
	}
	if err := json.Unmarshal(component.Props["action"], &action); err != nil {
		return RendererMessage{}, err
	}
	if action.Event == nil || action.Event.Name == "" {
		return RendererMessage{}, fmt.Errorf("component %q has no event", componentID)
	}
	context := make(map[string]any, len(action.Event.Context))
	for key, item := range action.Event.Context {
		value, err := s.data.Resolve(item, scope)
		if err != nil {
			return RendererMessage{}, fmt.Errorf("action context %q: %w", key, err)
		}
		context[key] = value
	}
	var userMessage string
	if action.Event.UserMessage != nil {
		value, err := s.data.Resolve(action.Event.UserMessage, scope)
		if err != nil {
			return RendererMessage{}, err
		}
		userMessage, ok = value.(string)
		if !ok {
			return RendererMessage{}, fmt.Errorf("userMessage must resolve to a string")
		}
	}
	message := RendererMessage{Version: Version, Action: &ActionEvent{
		Name: action.Event.Name, SurfaceID: s.id, SourceComponentID: componentID,
		Timestamp: now, Context: context, UserMessage: userMessage, Metadata: metadata,
	}}
	if metadata != nil {
		raw, err := json.Marshal(message)
		if err != nil {
			return RendererMessage{}, err
		}
		if err := ValidateProtocol("renderer_to_agent.json", raw); err != nil {
			return RendererMessage{}, err
		}
	}
	return message, nil
}
