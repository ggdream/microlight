package a2ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"uuid"
)

type FunctionHandler func(context.Context, *Surface, map[string]any) (any, error)
type FunctionRegistry map[string]map[string]FunctionHandler

// RPC pairs renderer calls with agent responses and enforces catalog permissions.
type RPC struct {
	Processor *Processor
	Functions FunctionRegistry
	Send      func(RendererMessage) error
	Timeout   time.Duration

	mu      sync.Mutex
	pending map[string]pendingCall
}

type pendingCall struct {
	surface string
	result  chan FunctionResponse
}

func (r *RPC) CallAgent(ctx context.Context, surface string, call FunctionCall) (json.RawMessage, error) {
	if r.Send == nil || surface == "" || call.Catalog == "" || call.Name == "" {
		return nil, fmt.Errorf("invalid agent function call")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := uuid.NewV7().String()
	pending := pendingCall{surface, make(chan FunctionResponse, 1)}
	r.mu.Lock()
	if r.pending == nil {
		r.pending = make(map[string]pendingCall)
	}
	r.pending[id] = pending
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, id); r.mu.Unlock() }()
	if err := r.Send(RendererMessage{Version: Version, CallAgent: &CallAgentFunction{SurfaceRef: SurfaceRef{ID: surface}, ID: id, Function: call}}); err != nil {
		return nil, err
	}
	select {
	case response := <-pending.result:
		if response.Error != nil {
			return nil, fmt.Errorf("%s: %s", response.Error.Code, response.Error.Message)
		}
		return response.Value, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *RPC) Resolve(response FunctionResponse) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, exists := r.pending[response.ID]
	if !exists {
		return false
	}
	delete(r.pending, response.ID)
	pending.result <- response
	return true
}

func (r *RPC) CancelSurface(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, pending := range r.pending {
		if id == "" || pending.surface == id {
			delete(r.pending, key)
			pending.result <- FunctionResponse{ID: key, Error: &FunctionError{Code: "CANCELLED", Message: "A2UI surface was removed"}}
		}
	}
}

func (r *RPC) Handle(ctx context.Context, message AgentMessage) error {
	if err := message.validate(); err != nil {
		return err
	}
	switch {
	case message.AgentResponse != nil:
		r.Resolve(*message.AgentResponse)
		return nil
	case message.CallRenderer != nil:
		if r.Send == nil {
			return fmt.Errorf("RPC has no renderer transport")
		}
		response := r.runRenderer(ctx, *message.CallRenderer)
		return r.Send(RendererMessage{Version: Version, RendererResponse: &response})
	default:
		if r.Processor == nil {
			return fmt.Errorf("RPC has no processor")
		}
		if err := r.Processor.Apply(message); err != nil {
			return err
		}
		if message.Delete != nil {
			r.CancelSurface(message.Delete.ID)
		}
		return nil
	}
}

func (r *RPC) runRenderer(ctx context.Context, request CallRendererFunction) FunctionResponse {
	response := FunctionResponse{ID: request.ID}
	fail := func(code, message string) FunctionResponse {
		response.Error = &FunctionError{Code: code, Message: message}
		return response
	}
	if r.Processor == nil {
		return fail("INVALID_FUNCTION_CALL", "renderer has no catalog registry")
	}
	surface := r.Processor.Surface(request.Surface)
	if request.Surface != "" && surface == nil {
		return fail("INVALID_FUNCTION_CALL", "unknown surface")
	}
	catalog, err := r.Processor.catalog(request.Function.Catalog)
	if err != nil {
		return fail("INVALID_FUNCTION_CALL", err.Error())
	}
	handler := r.Functions[catalog.ID][request.Function.Name]
	if handler == nil || !catalog.AgentMayCall(request.Function.Name) {
		return fail("INVALID_FUNCTION_CALL", "function unavailable to the agent")
	}
	if err := catalog.ValidateFunction(request.Function); err != nil {
		return fail("INVALID_FUNCTION_CALL", err.Error())
	}
	var data Model
	if surface != nil {
		data = *surface.Data()
	}
	var args map[string]any
	if request.Function.Args != nil {
		value, err := data.Resolve(request.Function.Args, "")
		if err != nil {
			return fail("EXECUTION_ERROR", err.Error())
		}
		args, _ = value.(map[string]any)
	}
	value, err := handler(ctx, surface, args)
	if err != nil {
		return fail("EXECUTION_ERROR", err.Error())
	}
	response.Value, err = json.Marshal(value)
	if err != nil {
		return fail("EXECUTION_ERROR", err.Error())
	}
	return response
}
