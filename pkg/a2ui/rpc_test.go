package a2ui_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
)

func TestBidirectionalRPCAndUserActivation(t *testing.T) {
	catalog, err := a2ui.ParseCatalog([]byte(`{"protocolVersion":"1.0","catalogId":"test","functions":{"read":{"allowedCallers":"rendererOrAgent","type":"object","properties":{"@call":{"const":"read"},"args":{"type":"object"}},"required":["@call"]},"write":{"allowedCallers":"rendererOrAgent","requiresUserActivation":true,"type":"object"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var p a2ui.Processor
	p.RegisterCatalog(catalog)
	if err := p.Apply(a2ui.AgentMessage{Version: a2ui.Version, Create: &a2ui.CreateSurface{SurfaceRef: a2ui.SurfaceRef{ID: "s"}, Catalog: "test"}}); err != nil {
		t.Fatal(err)
	}
	rpc := a2ui.RPC{Processor: &p}
	rpc.Functions = a2ui.FunctionRegistry{"test": {
		"read": func(_ context.Context, s *a2ui.Surface, _ map[string]any) (any, error) { return s.ID(), nil },
		"write": func(_ context.Context, _ *a2ui.Surface, _ map[string]any) (any, error) {
			t.Fatal("executed an activation-only function remotely")
			return nil, nil
		},
	}}
	rpc.Send = func(message a2ui.RendererMessage) error {
		wire, _ := json.Marshal(message)
		if _, err := a2ui.DecodeRenderer(wire); err != nil {
			return err
		}
		switch {
		case message.CallAgent != nil:
			reply := a2uiagent.Respond(t.Context(), []*a2ui.Catalog{catalog}, map[string]map[string]a2uiagent.AgentFunction{
				"test": {"read": func(context.Context, string, map[string]any) (any, error) { return "agent", nil }},
			}, *message.CallAgent)
			return rpc.Handle(t.Context(), reply)
		case message.RendererResponse.ID == "allowed":
			if string(message.RendererResponse.Value) != `"s"` {
				t.Fatal("incorrect renderer response")
			}
		default:
			if message.RendererResponse.Error == nil || message.RendererResponse.Error.Code != "INVALID_FUNCTION_CALL" {
				t.Fatal("activation guard failed")
			}
		}
		return nil
	}
	value, err := rpc.CallAgent(t.Context(), "s", a2ui.FunctionCall{Catalog: "test", Name: "read"})
	if err != nil || string(value) != `"agent"` {
		t.Fatalf("agent call returned %s, %v", value, err)
	}
	for id, name := range map[string]string{"allowed": "read", "denied": "write"} {
		message := a2ui.AgentMessage{Version: a2ui.Version, CallRenderer: &a2ui.CallRendererFunction{ID: id, Surface: "s", Function: a2ui.FunctionCall{Catalog: "test", Name: name}}}
		if err := rpc.Handle(t.Context(), message); err != nil {
			t.Fatal(err)
		}
	}
	var browser a2ui.Processor
	err = a2ui.ReadJSONL(strings.NewReader(`{"version":"1.0","createSurface":{"surfaceId":"links","catalogId":"https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json","components":[{"id":"root","component":"Button","child":"label","action":{"functionCall":{"@call":"openUrl","args":{"url":"https://example.com"}}}},{"id":"label","component":"Text","text":"Open"}]}}`), browser.Apply)
	if err != nil {
		t.Fatal(err)
	}
	var opened string
	event, err := browser.Surface("links").Activate("root", "", time.Now(), func(url string) error { opened = url; return nil })
	if err != nil || event != nil || opened != "https://example.com" {
		t.Fatalf("activation opened %q, event=%+v, err=%v", opened, event, err)
	}
}
