package workspace

import (
	"context"
	"errors"
	"testing"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/cloudwego/eino/adk/filesystem"
)

type desktopProbeFailure struct {
	ComputerDriver
	err error
}

func (d desktopProbeFailure) GetCapabilities(context.Context) (ComputerCapabilities, error) {
	return ComputerCapabilities{}, d.err
}

type failingDesktopRuntime struct {
	Runtime
	filesystem.Shell
	desktopProbeFailure
}

func (r failingDesktopRuntime) Computer() ComputerDriver { return r.desktopProbeFailure }

func TestDesktopProbeFailurePreservesShell(t *testing.T) {
	errUnavailable := errors.New("desktop permission probe failed")
	runtime := failingDesktopRuntime{desktopProbeFailure: desktopProbeFailure{err: errUnavailable}}
	ops, err := DiscoverOperations(t.Context(), runtime)
	if err != nil || ops.Shell == nil || !ops.Capabilities.Shell || ops.Computer != nil || ops.Capabilities.Computer != (ComputerCapabilities{}) {
		t.Fatalf("operations = %+v, error = %v", ops, err)
	}
}

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
