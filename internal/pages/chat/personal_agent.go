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
	open := agent.connect
	if open == nil {
		open = workspace.Open
		if agent.target.Kind == "" {
			native := localtools.LocalRuntime{
				Directory:     localtools.Workspace(ctx, agent.backend.config.Workspace),
				BrowserDriver: browser.driver, DesktopDriver: computer.driver,
			}
			open = func(context.Context, workspace.Target, string) (workspace.Runtime, error) { return native, nil }
		}
	}
	probe, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	executor, err := open(probe, agent.target, agent.secret)
	if err != nil {
		return nil, &workspaceConnectionError{agent.target, err}
	}
	defer executor.Close()
	ops, err := workspace.DiscoverOperations(probe, executor)
	cancel()
	if err != nil {
		return nil, &workspaceConnectionError{agent.target, err}
	}
	ctx = localtools.WithRuntime(ctx, agent.access, executor, agent.target, ops)
	if _, native := executor.(localtools.LocalRuntime); !native {
		ctx = localtools.WithWorkspaceReconnect(ctx, func(ctx context.Context) (workspace.Runtime, error) {
			return open(ctx, agent.target, agent.secret)
		})
		browser, computer = browserBinding{}, computerBinding{}
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
