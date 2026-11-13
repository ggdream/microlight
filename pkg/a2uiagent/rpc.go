package a2uiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ggdream/microlight/pkg/a2ui"
)

// AgentFunction receives a renderer-initiated call with resolved arguments.
type AgentFunction func(context.Context, string, map[string]any) (any, error)

func Respond(ctx context.Context, catalogs []*a2ui.Catalog, handlers map[string]map[string]AgentFunction, request a2ui.CallAgentFunction) a2ui.AgentMessage {
	result := a2ui.FunctionResponse{ID: request.ID}
	message := a2ui.AgentMessage{Version: a2ui.Version, AgentResponse: &result}
	fail := func(code, detail string) a2ui.AgentMessage {
		result.Error = &a2ui.FunctionError{Code: code, Message: detail}
		return message
	}
	if request.ID == "" || request.SurfaceRef.ID == "" || request.Function.Name == "" {
		return fail("INVALID_FUNCTION_CALL", "missing function name, call ID, or surface ID")
	}
	var selected *a2ui.Catalog
	for _, catalog := range catalogs {
		if catalog == nil || request.Function.Catalog != "" && catalog.ID != request.Function.Catalog {
			continue
		}
		if handlers[catalog.ID][request.Function.Name] != nil {
			if selected != nil {
				return fail("INVALID_FUNCTION_CALL", "function name matches more than one catalog")
			}
			selected = catalog
		}
	}
	if selected == nil || !selected.RendererMayCall(request.Function.Name) {
		return fail("UNKNOWN_FUNCTION", fmt.Sprintf("unknown agent function %s", request.Function.Name))
	}
	args, err := selected.FunctionArguments(request.Function)
	if err != nil {
		return fail("INVALID_FUNCTION_CALL", fmt.Sprintf("%s: %v", request.Function.Name, err))
	}
	request.Function.Catalog = selected.ID
	ctx = context.WithValue(ctx, functionCallKey{}, request)
	value, err := handlers[selected.ID][request.Function.Name](ctx, request.SurfaceRef.ID, args)
	if err != nil {
		var typed *a2ui.FunctionError
		if errors.As(err, &typed) && typed.Code != "" {
			return fail(typed.Code, typed.Message)
		}
		return fail("EXECUTION_ERROR", err.Error())
	}
	result.Value, err = json.Marshal(value)
	if err != nil {
		return fail("EXECUTION_ERROR", err.Error())
	}
	return message
}

// HandleFunctionMessage preserves correlation even when a renderer's call has
// malformed arguments. An envelope without a call ID cannot be answered.
func HandleFunctionMessage(ctx context.Context, catalogs []*a2ui.Catalog, handlers map[string]map[string]AgentFunction, raw []byte) (a2ui.AgentMessage, error) {
	var envelope a2ui.RendererMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Version != a2ui.Version ||
		envelope.CallAgent == nil || envelope.CallAgent.ID == "" {
		return a2ui.AgentMessage{}, fmt.Errorf("invalid A2UI function call envelope")
	}
	if _, err := a2ui.DecodeRenderer(raw); err != nil {
		return a2ui.AgentMessage{Version: a2ui.Version, AgentResponse: &a2ui.FunctionResponse{
			ID: envelope.CallAgent.ID, Error: &a2ui.FunctionError{Code: "INVALID_FUNCTION_CALL", Message: err.Error()},
		}}, nil
	}
	return Respond(ctx, catalogs, handlers, *envelope.CallAgent), nil
}

type functionCallKey struct{}

// FunctionCallFromContext returns the invocation metadata for a registered handler.
func FunctionCallFromContext(ctx context.Context) (a2ui.CallAgentFunction, bool) {
	call, ok := ctx.Value(functionCallKey{}).(a2ui.CallAgentFunction)
	return call, ok
}
