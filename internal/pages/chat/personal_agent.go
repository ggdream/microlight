package chat

import (
	"context"
	"fmt"
	"time"

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

func (agent PersonalAgent) run(ctx context.Context, store *localStore, sessionID string, input []*turn, resume *clarification, answer string, emit func(replyDelta) error) (*clarification, error) {
	browser := agent.browser
	computer := agent.computer
	if agent.target.Kind != "" {
		browser = browserBinding{}
		computer = computerBinding{}
		open := agent.connect
		if open == nil {
			open = workspace.Open
		}
		connect, cancel := context.WithTimeout(ctx, 20*time.Second)
		runtime, err := open(connect, agent.target, agent.secret)
		if err != nil {
			cancel()
			return nil, &workspaceConnectionError{agent.target, err}
		}
		defer runtime.Close()
		capabilities, err := workspace.DiscoverCapabilities(connect, runtime)
		cancel()
		if err != nil {
			return nil, &workspaceConnectionError{agent.target, err}
		}
		ctx = localtools.WithRuntime(ctx, agent.access, runtime, agent.target, capabilities)
		ctx = localtools.WithWorkspaceReconnect(ctx, func(ctx context.Context) (workspace.Runtime, error) {
			return open(ctx, agent.target, agent.secret)
		})
		if capabilities.Files {
			ctx = workspace.WithRuntimeInstructions(ctx, runtime, agent.target.Directory)
		}
		if capabilities.Browser {
			driver := runtime.(workspace.BrowserProvider).Browser()
			identity := ""
			if session, ok := driver.(workspace.BrowserSession); ok {
				identity = session.BrowserSessionID()
			}
			if identity == "" {
				identity = newID()
			}
			browser = browserBinding{driver: driver, identity: agent.target.Identity() + "/" + identity, label: agent.target.Label()}
		}
		if capabilities.Computer != (workspace.ComputerCapabilities{}) {
			driver := runtime.(workspace.ComputerProvider).Computer()
			identity := newID()
			if session, ok := driver.(workspace.ComputerSession); ok && session.ComputerSessionID() != "" {
				identity = session.ComputerSessionID()
			}
			computer = computerBinding{driver: driver, capabilities: capabilities.Computer, identity: agent.target.Identity() + "/" + identity, label: agent.target.Label()}
		}
	} else if computer.driver != nil {
		capabilities, err := computer.driver.GetCapabilities(ctx)
		if err != nil {
			return nil, err
		}
		computer.capabilities = capabilities
	}
	ctx = context.WithValue(ctx, browserContextKey{}, browser)
	ctx = context.WithValue(ctx, computerContextKey{}, computer)
	return agent.backend.runSession(ctx, store, sessionID, input, resume, answer, emit)
}
