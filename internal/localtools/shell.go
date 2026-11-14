package localtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ggdream/microlight/internal/harness"
	"github.com/ggdream/microlight/internal/shell"
	"github.com/ggdream/microlight/internal/workspace"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/cloudwego/eino/adk/filesystem"
	filesystemmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/schema"
)

const Execute = filesystemmw.ToolNameExecute

type localCommandShell struct{}

type reconnectWorkspaceKey struct{}
type commandJournalKey struct{}
type commandReattachmentKey struct{}
type commandCheckpointKey struct{}

const ContinueCommand = "continue-remote-command"

type CommandCheckpoint struct {
	ID, InterruptID, CallID string
	Automatic               bool
}

func WithCommandCheckpoint(ctx context.Context, checkpoint CommandCheckpoint) context.Context {
	return context.WithValue(ctx, commandCheckpointKey{}, checkpoint)
}

// WithCommandReattachment binds a restored approval to its existing task.
func WithCommandReattachment(ctx context.Context, record RemoteCommand) context.Context {
	return context.WithValue(ctx, commandReattachmentKey{}, record)
}

type RemoteCommand struct {
	ID, CallID, Command string
}

type CommandJournal struct {
	Record func(context.Context, RemoteCommand) error
	Finish func(context.Context, string, *backgroundtool.Outcome) error
}

func CheckpointForCommand(ctx context.Context, callID string) CommandCheckpoint {
	checkpoint, _ := ctx.Value(commandCheckpointKey{}).(CommandCheckpoint)
	if checkpoint.CallID != callID {
		return CommandCheckpoint{}
	}
	return checkpoint
}

// WithCommandJournal records the approved command before the executor can
// start it, including when the launch response is lost.
func WithCommandJournal(ctx context.Context, journal CommandJournal) context.Context {
	return context.WithValue(ctx, commandJournalKey{}, journal)
}

var ErrStopReply = errors.New("user stopped reply")

func WithWorkspaceReconnect(ctx context.Context, reconnect func(context.Context) (workspace.Runtime, error)) context.Context {
	return context.WithValue(ctx, reconnectWorkspaceKey{}, reconnect)
}

func ShellMiddleware(ctx context.Context) (adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], error) {
	description := "Run a non-interactive shell command in the conversation's selected execution environment. Use that environment's shell syntax. The working directory starts at the conversation workspace, or the local user's home for a local chat without a workspace. Shell state does not persist between calls. Command output is limited to 64 KiB."
	return filesystemmw.NewTyped[*schema.AgenticMessage](ctx, &filesystemmw.MiddlewareConfig{
		Shell:             localCommandShell{},
		ExecuteToolConfig: &filesystemmw.ExecuteToolConfig{Desc: &description},
	})
}

func (localCommandShell) Execute(ctx context.Context, input *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	op, ok := ctx.Value(approvedOperationKey{}).(localOperation)
	if !ok || op.name != Execute || input == nil || input.Command != op.content {
		return nil, errors.New("shell command was not authorized")
	}
	var response *filesystem.ExecuteResponse
	var err error
	var commands backgroundshell.RecoverableShell
	if provider, ok := op.runtime.(workspace.CommandProvider); ok {
		commands = provider.Commands()
	}
	record, restoring := ctx.Value(commandReattachmentKey{}).(RemoteCommand)
	checkpoint := CheckpointForCommand(ctx, op.callID)
	if commands == nil && ((restoring && record.ID == harness.OperationScope(ctx)+"/"+op.callID) || checkpoint.ID != "") {
		return nil, errors.New("recoverable shell capability is unavailable")
	}
	switch {
	case commands != nil && harness.OperationScope(ctx) != "" && op.callID != "":
		response, err = executeRecoverable(ctx, commands, op)
	case op.runtime != nil:
		response, err = op.runtime.(filesystem.Shell).Execute(ctx, input)
	default:
		response, err = shell.Execute(ctx, op.content, op.target, input.Timeout)
	}
	if report, ok := ctx.Value(reportToolResultKey{}).(func(Result)); ok && op.callID != "" && ctx.Err() == nil {
		result := Result{CallID: op.callID, Command: &CommandResult{Failed: err != nil}}
		if response != nil {
			result.Text = response.Output
			result.Command.ExitCode = response.ExitCode
			result.Command.TimedOut = response.TimedOut
			result.Command.Truncated = response.Truncated
		} else if err != nil {
			result.Text = err.Error()
		}
		report(result)
	}
	return response, err
}

func executeRecoverable(ctx context.Context, commands backgroundshell.RecoverableShell, op localOperation) (*filesystem.ExecuteResponse, error) {
	id := harness.OperationScope(ctx) + "/" + op.callID
	record, restoring := ctx.Value(commandReattachmentKey{}).(RemoteCommand)
	reattach := restoring && record.ID == id
	if reattach && record.Command != op.content {
		return nil, errors.New("restored command differs from the approved command")
	}
	journal, _ := ctx.Value(commandJournalKey{}).(CommandJournal)
	if journal.Record != nil {
		if err := journal.Record(ctx, RemoteCommand{ID: id, CallID: op.callID, Command: op.content}); err != nil {
			return nil, fmt.Errorf("record remote command before launch: %w", err)
		}
	}
	request := &backgroundshell.StartCommandRequest{TaskID: id, Command: op.content, Attempt: 1}
	started := reattach
	wait := func(commands backgroundshell.RecoverableShell) (*filesystem.ExecuteResponse, error) {
		var run backgroundtool.Run
		var err error
		if started {
			run, err = commands.RecoverCommand(ctx, &backgroundshell.RecoverCommandRequest{
				TaskID: id, Command: op.content, Attempt: request.Attempt,
			})
		} else {
			run, err = commands.StartCommand(ctx, request)
		}
		if err != nil {
			return nil, err
		}
		started = true
		outcome, err := run.Wait(ctx)
		if err != nil {
			return nil, err
		}
		if journal.Finish != nil {
			if err := journal.Finish(ctx, id, outcome); err != nil {
				return nil, err
			}
		}
		switch outcome.Status {
		case backgroundtask.StatusCompleted:
			var response filesystem.ExecuteResponse
			err = json.Unmarshal(outcome.Data, &response)
			return &response, err
		case backgroundtask.StatusCanceled:
			if reattach {
				return &filesystem.ExecuteResponse{Output: "Remote command canceled."}, nil
			}
			return nil, context.Canceled
		default:
			return nil, errors.New(outcome.Error)
		}
	}
	response, err := wait(commands)
	reconnect, _ := ctx.Value(reconnectWorkspaceKey{}).(func(context.Context) (workspace.Runtime, error))
	var transport *workspace.CommandTransportError
	if reconnect != nil && ctx.Err() == nil && errors.As(err, &transport) {
		openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		runtime, openErr := reconnect(openCtx)
		cancel()
		if openErr == nil {
			defer runtime.Close()
			if provider, ok := runtime.(workspace.CommandProvider); ok && provider.Commands() != nil {
				request.Attempt++
				response, err = wait(provider.Commands())
			}
		}
	}
	if errors.Is(context.Cause(ctx), ErrStopReply) && err != nil && reconnect != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		outcome, err := RecoverRemoteCommand(stopCtx, reconnect, RemoteCommand{ID: id, Command: op.content}, true)
		if err != nil {
			return nil, fmt.Errorf("remote command %s cancellation could not be confirmed: %w", id, err)
		}
		if journal.Finish != nil {
			if err := journal.Finish(stopCtx, id, outcome); err != nil {
				return nil, err
			}
		}
	}
	return response, err
}

func RecoverRemoteCommand(ctx context.Context, connect func(context.Context) (workspace.Runtime, error), record RemoteCommand, stop bool) (*backgroundtool.Outcome, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	runtime, err := connect(connectCtx)
	cancel()
	if err != nil {
		return nil, err
	}
	defer runtime.Close()
	provider, ok := runtime.(workspace.CommandProvider)
	if !ok || provider.Commands() == nil {
		return nil, errors.New("recoverable shell capability is unavailable")
	}
	commands := provider.Commands()
	req := &backgroundshell.RecoverCommandRequest{
		TaskID: record.ID, Command: record.Command, Attempt: 1,
	}
	if stopper, ok := commands.(workspace.CommandStopper); stop && ok {
		run, err := stopper.StopCommand(ctx, req)
		if err != nil {
			return nil, err
		}
		return run.Wait(ctx)
	}
	run, err := commands.RecoverCommand(ctx, req)
	if err != nil {
		return nil, err
	}
	if stop {
		if err := run.Stop(ctx); err != nil {
			return nil, err
		}
	}
	return run.Wait(ctx)
}
