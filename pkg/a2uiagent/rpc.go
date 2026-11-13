package a2uiagent

import (
	"context"
	"encoding/json"
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
	for _, catalog := range catalogs {
		if catalog.ID != request.Function.Catalog {
			continue
		}
		handler := handlers[catalog.ID][request.Function.Name]
		if handler == nil {
			break
		}
		if err := catalog.ValidateFunction(request.Function); err != nil {
			return fail("INVALID_FUNCTION_CALL", err.Error())
		}
		value, err := handler(ctx, request.SurfaceRef.ID, request.Function.Args)
		if err == nil {
			result.Value, err = json.Marshal(value)
		}
		if err != nil {
			return fail("EXECUTION_ERROR", err.Error())
		}
		return message
	}
	return fail("INVALID_FUNCTION_CALL", fmt.Sprintf("unknown agent function %s", request.Function.Name))
}
