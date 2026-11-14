package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/ggdream/microlight/internal/harness"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/workspace"
)

type PersonalAgent struct {
	backend  *Chat
	target   workspace.Target
	access   *localtools.Access
	secret   string
	connect  workspace.ConnectFunc
	browser  browserBinding
	computer computerBinding
}

type workspaceConnectionError struct {
	target workspace.Target
	cause  error
}

func (e *workspaceConnectionError) Error() string {
	return fmt.Sprintf("%s · %s", e.target.Label(), e.cause)
}
func (e *workspaceConnectionError) Unwrap() error { return e.cause }

func (agent PersonalAgent) run(ctx context.Context, store harness.Store, sessionID string, input []*turn, resume *clarification, answer string, emit func(replyDelta) error) (*clarification, error) {
	browser := agent.browser
	computer := agent.computer
	var executor workspace.Runtime
	probe, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if agent.target.Kind == "" {
		directory := localtools.Workspace(ctx, agent.backend.config.Workspace)
		executor = localtools.LocalRuntime{Directory: directory, BrowserDriver: browser.driver, DesktopDriver: computer.driver}
	} else {
		browser = browserBinding{}
		computer = computerBinding{}
		open := agent.connect
		if open == nil {
			open = workspace.Open
		}
		runtime, err := open(probe, agent.target, agent.secret)
		if err != nil {
			return nil, &workspaceConnectionError{agent.target, err}
		}
		executor = runtime
		ctx = localtools.WithWorkspaceReconnect(ctx, func(ctx context.Context) (workspace.Runtime, error) {
			return open(ctx, agent.target, agent.secret)
		})
	}
	defer executor.Close()
	ops, err := workspace.DiscoverOperations(probe, executor)
	cancel()
	if err != nil {
		return nil, &workspaceConnectionError{agent.target, err}
	}
	ctx = localtools.WithRuntime(ctx, agent.access, executor, agent.target, ops)
	if agent.target.Kind != "" {
		if ops.Files != nil {
			ctx = workspace.WithRuntimeInstructions(ctx, ops.Files, agent.target.Directory)
		}
		if driver := ops.Browser; driver != nil {
			identity := ""
			if session, ok := driver.(workspace.BrowserSession); ok {
				identity = session.BrowserSessionID()
			}
			if identity == "" {
				identity = newID()
			}
			browser = browserBinding{driver: driver, identity: agent.target.Identity() + "/" + identity, label: agent.target.Label()}
		}
		if driver := ops.Computer; ops.Capabilities.Computer != (workspace.ComputerCapabilities{}) {
			identity := newID()
			if session, ok := driver.(workspace.ComputerSession); ok && session.ComputerSessionID() != "" {
				identity = session.ComputerSessionID()
			}
			computer = computerBinding{driver: driver, capabilities: ops.Capabilities.Computer, identity: agent.target.Identity() + "/" + identity, label: agent.target.Label()}
		}
	} else {
		browser.driver = ops.Browser
		computer.driver, computer.capabilities = ops.Computer, ops.Capabilities.Computer
	}
	ctx = context.WithValue(ctx, browserContextKey{}, browser)
	ctx = context.WithValue(ctx, computerContextKey{}, computer)
	return agent.backend.runSession(ctx, store, sessionID, input, resume, answer, emit)
}
