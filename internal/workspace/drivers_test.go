package workspace

import (
	"context"
	"testing"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/cloudwego/eino/adk/filesystem"
)

type declaredShellRuntime struct {
	Runtime
	filesystem.Shell
	commandsCalled bool
}

func (*declaredShellRuntime) GetCapabilities(context.Context) (Capabilities, error) {
	return Capabilities{Shell: true}, nil
}

func (r *declaredShellRuntime) Commands() backgroundshell.RecoverableShell {
	r.commandsCalled = true
	return nil
}

func TestShellCapabilityDoesNotEnableCommandRecovery(t *testing.T) {
	runtime := &declaredShellRuntime{}
	ops, err := DiscoverOperations(t.Context(), runtime)
	if err != nil || ops.Shell == nil || !ops.Capabilities.Shell || ops.Capabilities.RecoverableShell {
		t.Fatalf("shell capabilities = %+v, %v", ops.Capabilities, err)
	}
	commands, err := DiscoverCommands(t.Context(), runtime)
	if err != nil || commands != nil || runtime.commandsCalled {
		t.Fatalf("recoverable command discovery = %v, %v; provider called = %t", commands, err, runtime.commandsCalled)
	}
}
