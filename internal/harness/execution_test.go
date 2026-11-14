package harness

import (
	"context"
	"errors"
	"testing"

	"github.com/ggdream/microlight/internal/workspace"
)

type executionFixture struct {
	workspace.Runtime
	probeError error
	closed     bool
}

func (r *executionFixture) Close() error {
	r.closed = true
	return nil
}

func (r *executionFixture) GetCapabilities(context.Context) (workspace.Capabilities, error) {
	return workspace.Capabilities{}, r.probeError
}

func TestOpenExecutionClosesRuntimeWhenDiscoveryFails(t *testing.T) {
	probeError := errors.New("capabilities unavailable")
	runtime := &executionFixture{probeError: probeError}
	connected, _, err := OpenExecution(t.Context(), func(ctx context.Context) (workspace.Runtime, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("connector has no deadline")
		}
		return runtime, nil
	})
	if connected != nil || !runtime.closed || !errors.Is(err, probeError) {
		t.Fatalf("connected = %v, closed = %t, error = %v", connected, runtime.closed, err)
	}
}
