package a2uimygo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/a2ui"
)

// AgentActionRPC resolves selected functions in button expressions through an
// A2UI transport. Dispatch runs off the UI thread. Local evaluation uses a
// snapshot, so a failed remote expression cannot mutate the live data model.
type AgentActionRPC struct {
	Transport     *a2ui.RPC
	IsRemote      func(a2ui.FunctionCall) bool
	EvaluateLocal func(*a2ui.Model, a2ui.FunctionCall, string) (any, error)
}

type actionOutcome struct {
	event         *a2ui.RendererMessage
	before, after any
	err           error
}

type pendingAction struct {
	result <-chan actionOutcome
	cancel context.CancelFunc
}

func (a *pendingAction) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

func (r renderer) actionRPC(button ui.Element) (*pendingAction, bool) {
	rpc := r.AgentRPC
	if rpc == nil || rpc.Transport == nil || rpc.IsRemote == nil {
		return nil, false
	}
	slot := ui.Local(button, "agent-action-rpc", func() *pendingAction { return new(pendingAction) })
	action := *slot
	select {
	case outcome := <-action.result:
		_ = action.Close()
		action.result, action.cancel = nil, nil
		if outcome.err == nil && !reflect.DeepEqual(outcome.before, outcome.after) {
			current, _ := r.surface.Data().Get("")
			if !reflect.DeepEqual(current, outcome.before) {
				outcome.err = fmt.Errorf("A2UI surface changed during action")
			} else if err := r.surface.Data().Set("", outcome.after); err != nil {
				outcome.err = err
			} else if r.OnDataChange != nil {
				r.OnDataChange("", outcome.after)
			}
		}
		if outcome.err != nil {
			if r.OnError != nil {
				r.OnError(outcome.err)
			}
			if r.OnRendererError != nil {
				id := r.surface.ID()
				report := a2ui.RendererError{Code: "EXPRESSION_ERROR", SurfaceID: &id, Message: outcome.err.Error()}
				var invocation *remoteActionError
				if errors.As(outcome.err, &invocation) {
					report.SurfaceID, report.FunctionCallID = nil, invocation.id
				}
				r.OnRendererError(a2ui.RendererMessage{Version: a2ui.Version, Error: &report})
			}
		} else if outcome.event != nil && r.OnAction != nil {
			r.OnAction(*outcome.event)
		}
	default:
	}
	return action, action.result != nil
}

type remoteActionError struct {
	id  string
	err error
}

func (e *remoteActionError) Error() string { return e.err.Error() }
func (e *remoteActionError) Unwrap() error { return e.err }

func (r renderer) dispatchRemoteAction(c *ui.Context, node a2ui.Component, scope string, action *pendingAction) bool {
	// Event actions and the built-in URL opener keep their normal UI path.
	var definition struct {
		Function json.RawMessage `json:"functionCall"`
	}
	if json.Unmarshal(node.Props["action"], &definition) != nil || len(definition.Function) == 0 {
		return false
	}
	rpc := r.AgentRPC
	if !r.hasRemoteFunction(definition.Function) {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan actionOutcome, 1)
	action.result, action.cancel = result, cancel
	snapshot := r.surface.Snapshot()
	model, id, now := snapshot.Data(), snapshot.ID(), c.Now()
	before, _ := r.surface.Data().Get("")
	services := c.Services()
	model.EvaluateFunction = func(call a2ui.FunctionCall, scope string) (any, error) {
		if !rpc.IsRemote(call) {
			if rpc.EvaluateLocal != nil {
				return rpc.EvaluateLocal(model, call, scope)
			}
			return model.EvaluateLocal(call, scope)
		}
		responses, stop, err := rpc.Transport.BeginAgentCall(ctx, id, call)
		if err != nil {
			return nil, err
		}
		defer stop()
		response := <-responses
		if response.Error != nil {
			return nil, &remoteActionError{id: response.ID, err: response.Error}
		}
		var value any
		err = json.Unmarshal(response.Value, &value)
		return value, err
	}
	go func() {
		event, err := snapshot.Activate(node.ID, scope, now, nil)
		after, _ := model.Get("")
		result <- actionOutcome{event: event, err: err, before: before, after: after}
		services.Invalidate()
	}()
	return true
}

func (r renderer) hasRemoteFunction(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	var remote func(any) bool
	remote = func(value any) bool {
		switch value := value.(type) {
		case map[string]any:
			name, _ := value["@call"].(string)
			if name != "" {
				catalog, _ := value["catalogId"].(string)
				if catalog == "" {
					catalog = r.surface.Catalog()
				}
				if r.AgentRPC.IsRemote(a2ui.FunctionCall{Name: name, Catalog: catalog}) {
					return true
				}
			}
			for _, child := range value {
				if remote(child) {
					return true
				}
			}
		case []any:
			for _, child := range value {
				if remote(child) {
					return true
				}
			}
		}
		return false
	}
	return remote(value)
}
