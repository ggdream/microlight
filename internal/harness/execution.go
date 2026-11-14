package harness

import (
	"context"
	"errors"
	"time"

	"github.com/ggdream/microlight/internal/workspace"
)

// OpenExecution connects to a host-selected environment and discovers its
// available tools. The caller owns the returned runtime.
func OpenExecution(ctx context.Context, open func(context.Context) (workspace.Runtime, error)) (workspace.Runtime, workspace.Operations, error) {
	if open == nil {
		return nil, workspace.Operations{}, errors.New("execution connector is required")
	}
	probe, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := probe.Err(); err != nil {
		return nil, workspace.Operations{}, err
	}
	runtime, err := open(probe)
	if err != nil {
		return nil, workspace.Operations{}, err
	}
	if runtime == nil {
		return nil, workspace.Operations{}, errors.New("execution connector returned no runtime")
	}
	operations, err := workspace.DiscoverOperations(probe, runtime)
	if err != nil {
		_ = runtime.Close()
		return nil, workspace.Operations{}, err
	}
	return runtime, operations, nil
}
