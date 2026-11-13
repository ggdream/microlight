package a2uiagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ggdream/microlight/pkg/a2ui"
)

type RendererCallbacks struct {
	Action           func(a2ui.ActionEvent) bool
	Error            func(a2ui.RendererError) bool
	RendererResponse func(a2ui.FunctionResponse) bool
}

// ProcessRendererMessage routes a renderer envelope. An unhandled event may
// be forwarded to the model. Agent function calls always get a direct answer.
func ProcessRendererMessage(ctx context.Context, raw []byte, catalogs []*a2ui.Catalog, handlers map[string]map[string]AgentFunction, callbacks RendererCallbacks) (bool, []a2ui.AgentMessage, error) {
	ids := make(map[string]bool, len(catalogs))
	for _, catalog := range catalogs {
		if catalog == nil || ids[catalog.ID] {
			return false, nil, fmt.Errorf("missing or duplicate A2UI catalog")
		}
		ids[catalog.ID] = true
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false, nil, err
	}
	var version string
	if len(fields) != 2 || json.Unmarshal(fields["version"], &version) != nil || version != a2ui.Version {
		return false, nil, fmt.Errorf("invalid A2UI renderer envelope")
	}
	switch {
	case fields["callAgentFunction"] != nil:
		reply, err := HandleFunctionMessage(ctx, catalogs, handlers, raw)
		if err != nil {
			return false, nil, err
		}
		return true, []a2ui.AgentMessage{reply}, nil
	case fields["action"] != nil:
		if callbacks.Action == nil {
			return false, nil, nil
		}
	case fields["error"] != nil:
		if callbacks.Error == nil {
			return false, nil, nil
		}
	case fields["rendererFunctionResponse"] != nil:
		if callbacks.RendererResponse == nil {
			return false, nil, nil
		}
	default:
		return false, nil, fmt.Errorf("unknown A2UI renderer message")
	}
	message, err := a2ui.DecodeRenderer(raw)
	if err != nil {
		return false, nil, err
	}
	switch {
	case message.Action != nil:
		return callbacks.Action(*message.Action), nil, nil
	case message.Error != nil:
		return callbacks.Error(*message.Error), nil, nil
	case message.RendererResponse != nil:
		return callbacks.RendererResponse(*message.RendererResponse), nil, nil
	default:
		return false, nil, fmt.Errorf("empty A2UI renderer message")
	}
}
