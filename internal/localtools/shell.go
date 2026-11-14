package localtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ggdream/microlight/internal/harness"
	"github.com/ggdream/microlight/internal/shell"
	"github.com/ggdream/microlight/internal/workspace"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/cloudwego/eino/adk/filesystem"
	filesystemmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
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

type commandReattachment struct {
	RemoteCommand
	outcome *backgroundtool.Outcome
}

// WithCommandReattachment binds a restored approval to its existing task and
// its terminal result, if the journal already received it.
func WithCommandReattachment(ctx context.Context, record RemoteCommand, outcome *backgroundtool.Outcome) context.Context {
	return context.WithValue(ctx, commandReattachmentKey{}, commandReattachment{record, outcome})
}

type RemoteCommand struct {
	ID, CallID, Command string
	LaunchConfirmed     bool
}

type CommandJournal struct {
	Record        func(context.Context, RemoteCommand) error
	ConfirmLaunch func(context.Context, string) error
	Finish        func(context.Context, string, *backgroundtool.Outcome) error
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

func ShellTool() (tool.BaseTool, error) {
	description := "Run a non-interactive shell command in the conversation's selected execution environment. Use that environment's shell syntax. The working directory starts at the conversation workspace, or the local user's home for a local chat without a workspace. Shell state does not persist between calls. Command output is limited to 64 KiB."
	return utils.InferTool(Execute, description, func(ctx context.Context, args struct {
		Command string `json:"command" jsonschema:"required"`
		Timeout int    `json:"timeout,omitempty" jsonschema_description:"Optional execution limit in seconds. Defaults to 120 and is capped at 120."`
	}) (string, error) {
		req := &filesystem.ExecuteRequest{Command: args.Command}
		if args.Timeout > 0 {
			timeout := time.Duration(min(args.Timeout, 120)) * time.Second
			req.Timeout = &timeout
		}
		response, err := (localCommandShell{}).Execute(ctx, req)
		if err != nil {
			return "", err
		}
		if response == nil {
			return "", nil
		}
		var parts []string
		if response.Output != "" {
			parts = append(parts, response.Output)
		}
		switch {
		case response.TimedOut:
			parts = append(parts, "[Command timed out and was stopped]")
		case response.ExitCode != nil && *response.ExitCode != 0:
			parts = append(parts, fmt.Sprintf("[Command failed with exit code %d]", *response.ExitCode))
		}
		if response.Truncated {
			parts = append(parts, "[Output was truncated due to size limits]")
		}
		if len(parts) == 0 {
			return "[Command executed successfully with no output]", nil
		}
		return strings.Join(parts, "\n"), nil
	})
}

func (localCommandShell) Execute(ctx context.Context, input *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	op, ok := ctx.Value(approvedOperationKey{}).(localOperation)
	if !ok || op.name != Execute || input == nil || input.Command != op.content {
		return nil, errors.New("shell command was not authorized")
	}
	var response *filesystem.ExecuteResponse
	var err error
	commands := op.operations.Commands
	record, restoring := ctx.Value(commandReattachmentKey{}).(commandReattachment)
	reattach := restoring && record.ID == harness.OperationScope(ctx)+"/"+op.callID && record.CallID == op.callID
	checkpoint := CheckpointForCommand(ctx, op.callID)
	switch {
	case reattach && record.Command != op.content:
		return nil, errors.New("restored command differs from the approved command")
	case reattach && record.outcome != nil:
		response, err = commandResponse(record.outcome, true)
	case commands == nil && (reattach || checkpoint.ID != ""):
		return nil, errors.New("recoverable shell capability is unavailable")
	case commands != nil && harness.OperationScope(ctx) != "" && op.callID != "":
		if input.Timeout != nil {
			ctx = workspace.WithCommandTimeout(ctx, *input.Timeout)
		}
		response, err = executeRecoverable(ctx, commands, op)
	case op.runtime != nil:
		if op.operations.Shell == nil {
			return nil, errors.New("command requires a recoverable session and tool call ID")
		}
		executor := op.operations.Shell
		if native, ok := executor.(LocalRuntime); ok {
			// Use the path resolved during approval.
			response, err = native.executeApprovedCommand(ctx, op.target, input)
		} else {
			response, err = executor.Execute(ctx, input)
		}
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
	record, restoring := ctx.Value(commandReattachmentKey{}).(commandReattachment)
	reattach := restoring && record.ID == id && record.CallID == op.callID
	journal, _ := ctx.Value(commandJournalKey{}).(CommandJournal)
	if journal.Record != nil {
		if err := journal.Record(ctx, RemoteCommand{ID: id, CallID: op.callID, Command: op.content}); err != nil {
			return nil, fmt.Errorf("record remote command before launch: %w", err)
		}
	}
	request := &backgroundshell.StartCommandRequest{TaskID: id, Command: op.content, Attempt: 1}
	recoverFirst := reattach
	// A journal entry is written before launch. Only a missing, unconfirmed
	// task can be retried here, and only on an executor that deduplicates starts.
	allowLaunchRetry := reattach && !record.LaunchConfirmed
	wait := func(commands backgroundshell.RecoverableShell) (*filesystem.ExecuteResponse, error) {
		var run backgroundtool.Run
		var err error
		if recoverFirst {
			run, err = commands.RecoverCommand(ctx, &backgroundshell.RecoverCommandRequest{
				TaskID: id, Command: op.content, Attempt: request.Attempt,
			})
			if starter, ok := commands.(workspace.IdempotentCommandStarter); ok && allowLaunchRetry && errors.Is(err, backgroundtask.ErrNotFound) && ctx.Err() == nil {
				run, err = starter.StartCommandOnce(ctx, request)
			}
		} else {
			run, err = commands.StartCommand(ctx, request)
		}
		if err != nil {
			return nil, err
		}
		recoverFirst = true
		allowLaunchRetry = false
		if journal.ConfirmLaunch != nil {
			if err := journal.ConfirmLaunch(ctx, id); err != nil {
				return nil, err
			}
		}
		outcome, err := run.Wait(ctx)
		if err != nil {
			return nil, err
		}
		if journal.Finish != nil {
			if err := journal.Finish(ctx, id, outcome); err != nil {
				return nil, err
			}
		}
		return commandResponse(outcome, reattach)
	}
	response, err := wait(commands)
	reconnect, _ := ctx.Value(reconnectWorkspaceKey{}).(func(context.Context) (workspace.Runtime, error))
	if reconnect != nil {
		if original, ok := op.runtime.(workspace.CommandSession); ok {
			open := reconnect
			reconnect = func(ctx context.Context) (workspace.Runtime, error) {
				rt, err := open(ctx)
				if err != nil {
					return nil, err
				}
				if session, ok := rt.(workspace.CommandSession); !ok || session.CommandSessionID() != original.CommandSessionID() {
					rt.Close()
					return nil, errors.New("execution environment command journal changed")
				}
				return rt, nil
			}
		}
	}
	var transport *workspace.CommandTransportError
	if reconnect != nil && ctx.Err() == nil && errors.As(err, &transport) {
		if !recoverFirst {
			// The executor may have accepted the launch before the connection
			// failed. Check its state before sending another start.
			recoverFirst, allowLaunchRetry = true, true
		}
		openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		runtime, openErr := reconnect(openCtx)
		if openErr == nil {
			defer runtime.Close()
			recovered, discoverErr := workspace.DiscoverCommands(openCtx, runtime)
			cancel()
			if discoverErr == nil && recovered != nil {
				request.Attempt++
				response, err = wait(recovered)
			}
		} else {
			cancel()
			err = errors.Join(err, openErr)
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

func commandResponse(outcome *backgroundtool.Outcome, reattach bool) (*filesystem.ExecuteResponse, error) {
	switch outcome.Status {
	case backgroundtask.StatusCompleted:
		var response filesystem.ExecuteResponse
		if err := json.Unmarshal(outcome.Data, &response); err != nil {
			return nil, err
		}
		return &response, nil
	case backgroundtask.StatusCanceled:
		if reattach {
			return &filesystem.ExecuteResponse{Output: "Remote command canceled."}, nil
		}
		return nil, context.Canceled
	default:
		return nil, errors.New(outcome.Error)
	}
}

func RecoverRemoteCommand(ctx context.Context, connect func(context.Context) (workspace.Runtime, error), record RemoteCommand, stop bool) (*backgroundtool.Outcome, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	runtime, err := connect(connectCtx)
	if err != nil {
		return nil, err
	}
	defer runtime.Close()
	commands, err := workspace.DiscoverCommands(connectCtx, runtime)
	cancel()
	if err != nil {
		return nil, err
	}
	if commands == nil {
		return nil, errors.New("recoverable shell capability is unavailable")
	}
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
