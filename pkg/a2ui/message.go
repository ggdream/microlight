// Package a2ui implements the A2UI 1.0 message format and surface state.
package a2ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const Version = "1.0"
const BasicCatalog = "https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json"
const Root = "root"

type AgentMessage struct {
	Version       string                `json:"version"`
	Create        *CreateSurface        `json:"createSurface,omitempty"`
	Components    *UpdateComponents     `json:"updateComponents,omitempty"`
	Data          *UpdateDataModel      `json:"updateDataModel,omitempty"`
	Delete        *SurfaceRef           `json:"deleteSurface,omitempty"`
	CallRenderer  *CallRendererFunction `json:"callRendererFunction,omitempty"`
	AgentResponse *FunctionResponse     `json:"agentFunctionResponse,omitempty"`
}

type SurfaceRef struct {
	ID string `json:"surfaceId"`
}

type CreateSurface struct {
	SurfaceRef
	Catalog       string         `json:"catalogId,omitempty"`
	SendDataModel bool           `json:"sendDataModel,omitempty"`
	Components    []Component    `json:"components,omitempty"`
	DataModel     map[string]any `json:"dataModel,omitempty"`
	Metadata      *Metadata      `json:"metadata,omitempty"`
}

type Metadata struct {
	Extensions map[string]any `json:"extensions"`
}

type UpdateComponents struct {
	SurfaceRef
	Components []Component `json:"components"`
}

type UpdateDataModel struct {
	SurfaceRef
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value"`
}

type CallRendererFunction struct {
	ID       string       `json:"functionCallId"`
	Surface  string       `json:"surfaceId,omitempty"`
	Function FunctionCall `json:"callFunction"`
}

type CallAgentFunction struct {
	SurfaceRef
	ID       string       `json:"functionCallId"`
	Function FunctionCall `json:"callFunction"`
}

type FunctionCall struct {
	Name    string         `json:"@call"`
	Catalog string         `json:"catalogId,omitempty"`
	Args    map[string]any `json:"args,omitempty"`
}

type FunctionResponse struct {
	ID    string          `json:"functionCallId"`
	Value json.RawMessage `json:"value,omitempty"`
	Error *FunctionError  `json:"error,omitempty"`
}

type FunctionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *FunctionError) Error() string { return e.Message }

type RendererMessage struct {
	Version          string             `json:"version"`
	Action           *ActionEvent       `json:"action,omitempty"`
	CallAgent        *CallAgentFunction `json:"callAgentFunction,omitempty"`
	RendererResponse *FunctionResponse  `json:"rendererFunctionResponse,omitempty"`
	Error            *RendererError     `json:"error,omitempty"`
}

type RendererError struct {
	Code           string  `json:"code"`
	Message        string  `json:"message"`
	SurfaceID      *string `json:"surfaceId,omitempty"`
	FunctionCallID string  `json:"functionCallId,omitempty"`
	Path           string  `json:"path,omitempty"`
}

type ActionEvent struct {
	Name              string         `json:"name"`
	SurfaceID         string         `json:"surfaceId"`
	SourceComponentID string         `json:"sourceComponentId"`
	Timestamp         time.Time      `json:"timestamp"`
	Context           map[string]any `json:"context"`
	UserMessage       string         `json:"userMessage,omitempty"`
	Metadata          *Metadata      `json:"metadata,omitempty"`
}

type Event struct {
	Name        string         `json:"name"`
	Context     map[string]any `json:"context,omitempty"`
	UserMessage any            `json:"userMessage,omitempty"`
}

type Binding struct {
	Path string `json:"@path"`
}

// Component keeps catalog-specific properties in their original JSON shape.
type Component struct {
	ID      string
	Type    string
	Catalog string
	Props   map[string]json.RawMessage
}

func NewComponent(id, kind string, props map[string]any) (Component, error) {
	raw := make(map[string]json.RawMessage, len(props))
	for key, value := range props {
		encoded, err := json.Marshal(value)
		if err != nil {
			return Component{}, fmt.Errorf("component %q property %q: %w", id, key, err)
		}
		raw[key] = encoded
	}
	return Component{ID: id, Type: kind, Props: raw}, nil
}

func (c Component) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(c.Props)+3)
	for key, value := range c.Props {
		fields[key] = value
	}
	fields["id"], _ = json.Marshal(c.ID)
	fields["component"], _ = json.Marshal(c.Type)
	if c.Catalog != "" {
		fields["@catalogId"], _ = json.Marshal(c.Catalog)
	}
	return json.Marshal(fields)
}

func (c *Component) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var next Component
	for key, target := range map[string]*string{"id": &next.ID, "component": &next.Type, "@catalogId": &next.Catalog} {
		if raw, ok := fields[key]; ok {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("component %s must be a string", key)
			}
			if err := json.Unmarshal(raw, target); err != nil {
				return fmt.Errorf("component %s: %w", key, err)
			}
			delete(fields, key)
		}
	}
	if next.ID == "" || next.Type == "" || next.Type == "Surface" {
		return fmt.Errorf("invalid component id or type")
	}
	next.Props = fields
	*c = next
	return nil
}

// Decode reads one agent message with the official 1.0 envelope.
func Decode(data []byte) (AgentMessage, error) {
	var message AgentMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil {
		return message, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return message, fmt.Errorf("expected a single A2UI message")
	}
	if err := ValidateProtocol("agent_to_renderer.json", data); err != nil {
		return message, err
	}
	return message, message.validate()
}

func (m AgentMessage) validate() error {
	if m.Version != Version {
		return fmt.Errorf("unsupported A2UI version %q", m.Version)
	}
	count := 0
	for _, present := range []bool{m.Create != nil, m.Components != nil, m.Data != nil,
		m.Delete != nil, m.CallRenderer != nil, m.AgentResponse != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("expected exactly one A2UI payload")
	}
	if m.Components != nil && len(m.Components.Components) == 0 {
		return fmt.Errorf("updateComponents requires components")
	}
	if m.Data != nil && len(m.Data.Value) == 0 {
		return fmt.Errorf("updateDataModel requires value")
	}
	if m.CallRenderer != nil && (m.CallRenderer.ID == "" || m.CallRenderer.Function.Name == "" || m.CallRenderer.Function.Catalog == "") {
		return fmt.Errorf("callRendererFunction requires an ID, function name and catalog")
	}
	if r := m.AgentResponse; r != nil && (r.ID == "" || (len(r.Value) == 0) == (r.Error == nil)) {
		return fmt.Errorf("agentFunctionResponse requires an ID and exactly one result")
	}
	return nil
}

// DecodeRenderer validates and decodes a renderer-to-agent event.
func DecodeRenderer(data []byte) (RendererMessage, error) {
	var message RendererMessage
	if err := ValidateProtocol("renderer_to_agent.json", data); err != nil {
		return message, err
	}
	err := json.Unmarshal(data, &message)
	return message, err
}

// DecodePermissive accepts deprecated v1.0 fields that a renderer may ignore.
// The returned message always uses the strict v1.0 representation.
func DecodePermissive(data []byte) (AgentMessage, error) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) == nil {
		var surface map[string]json.RawMessage
		if json.Unmarshal(envelope["createSurface"], &surface) == nil && surface["theme"] != nil {
			delete(surface, "theme")
			envelope["createSurface"], _ = json.Marshal(surface)
			data, _ = json.Marshal(envelope)
		}
	}
	return Decode(data)
}
