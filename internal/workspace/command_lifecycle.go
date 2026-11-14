package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/cloudwego/eino/adk/filesystem"
)

// CommandTransportError means the executor could not observe the authoritative
// command state. Retrying requires the original task ID.
type CommandTransportError struct{ cause error }

func (e *CommandTransportError) Error() string { return e.cause.Error() }
func (e *CommandTransportError) Unwrap() error { return e.cause }

type CommandProvider interface {
	Commands() backgroundshell.RecoverableShell
}

// IdempotentCommandStarter guarantees that starting a command with an
// existing task ID cannot run it again, including after a lost launch reply.
type IdempotentCommandStarter interface {
	StartCommandOnce(context.Context, *backgroundshell.StartCommandRequest) (backgroundtool.Run, error)
}

// CommandStopper can record cancellation even before a launch is acknowledged.
type CommandStopper interface {
	StopCommand(context.Context, *backgroundshell.RecoverCommandRequest) (backgroundtool.Run, error)
}

const maxCommandSeconds = 120

type commandTimeoutKey struct{}

// WithCommandTimeout adds a launch limit to Eino's recoverable shell request.
// Recovery and cancellation continue to use the original task ID.
func WithCommandTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, commandTimeoutKey{}, timeout)
}

func commandSeconds(ctx context.Context) int {
	timeout, _ := ctx.Value(commandTimeoutKey{}).(time.Duration)
	if timeout <= 0 {
		return maxCommandSeconds
	}
	timeout = min(timeout, maxCommandSeconds*time.Second)
	return int((timeout + time.Second - 1) / time.Second)
}

type remoteCommands struct {
	run       func(context.Context, string) (string, error)
	directory string
	scope     string
	launchErr error
}

func supportsRemoteCommands(ctx context.Context, run func(context.Context, string) (string, error)) bool {
	_, err := run(ctx, `set -eu
test -n "${HOME:-}" && test -w "$HOME"
for utility in nohup setsid flock mkfifo head date; do command -v "$utility" >/dev/null; done
`)
	return err == nil
}

func (c remoteCommands) StartCommand(ctx context.Context, req *backgroundshell.StartCommandRequest) (backgroundtool.Run, error) {
	return c.StartCommandOnce(ctx, req)
}

func (c remoteCommands) StartCommandOnce(ctx context.Context, req *backgroundshell.StartCommandRequest) (backgroundtool.Run, error) {
	if req == nil {
		return nil, errors.New("command request is required")
	}
	task, err := c.task(req.TaskID, req.Command)
	if err != nil {
		return nil, err
	}
	if c.launchErr != nil {
		if _, err := task.observe(ctx); errors.Is(err, backgroundtask.ErrNotFound) {
			return nil, c.launchErr
		} else if err != nil {
			return nil, err
		}
		return task, nil
	}
	// The remote worker owns the claim. A dropped launch response can be
	// retried with the same ID without executing the command again.
	err = task.launch(ctx, req.Command)
	if err != nil {
		return nil, err
	}
	if _, err := task.observe(ctx); err != nil {
		return nil, err
	}
	return task, nil
}

func (c remoteCommands) RecoverCommand(ctx context.Context, req *backgroundshell.RecoverCommandRequest) (backgroundtool.Run, error) {
	if req == nil {
		return nil, errors.New("command request is required")
	}
	task, err := c.task(req.TaskID, req.Command)
	if err != nil {
		return nil, err
	}
	if _, err := task.observe(ctx); err != nil {
		return nil, err
	}
	return task, nil
}

func (c remoteCommands) StopCommand(ctx context.Context, req *backgroundshell.RecoverCommandRequest) (backgroundtool.Run, error) {
	if req == nil {
		return nil, errors.New("command request is required")
	}
	task, err := c.task(req.TaskID, req.Command)
	if err != nil {
		return nil, err
	}
	if err := task.Stop(ctx); err != nil {
		return nil, err
	}
	return task, nil
}

type remoteCommand struct {
	remoteCommands
	key, signature string
}

func (c remoteCommands) task(id, command string) (*remoteCommand, error) {
	if id == "" || len(id) > 256 || strings.TrimSpace(command) == "" || len(command) > 16<<10 || strings.ContainsRune(command, 0) {
		return nil, errors.New("invalid task ID or command")
	}
	return &remoteCommand{
		remoteCommands: c,
		key:            fmt.Sprintf("%x", sha256.Sum256([]byte(id))),
		signature:      fmt.Sprintf("%x", sha256.Sum256([]byte(command))),
	}, nil
}

const commandPreamble = `set -eu
umask 077
base="${XDG_CACHE_HOME:-${HOME:?}/.cache}/microlight/commands"
`

func (r *remoteCommand) script(body string) string {
	return commandPreamble + `task="$base/` + r.scope + "/" + r.key + `"
signature=` + r.signature + "\n" + body
}

// Each worker atomically publishes the command digest and terminal state.
// Losing its lock without a result is an unknown outcome and must never
// trigger an automatic re-execution of the same task.
const commandWorker = `set -eu
task=$1
signature=$2
directory=$3
command=$4
timeout=$5
cd "$task"
exec 8>launch
flock 8
[ ! -f signature ] && [ ! -f canceled-before-launch ] || exit 0
exec 9>lease
flock -n 9 || exit 0

# Keep only the first 64 KiB and drain the rest without retaining it.
rm -f fifo
mkfifo fifo
setsid /bin/sh -c 'head -c 65537; cat > /dev/null' < fifo > output 8>&- 9>&- &
reader=$!

# Start the command in its own process group. Keep the supervisor outside
# that group, so it can record the result after canceling the command.
printf '%s\n' "$signature" > signature.tmp
mv signature.tmp signature
setsid /bin/sh -c '
    cd "$1" || exit 125
    exec /bin/sh -c "$2"
' sh "$directory" "$command" < /dev/null > fifo 2>&1 8>&- 9>&- &
pid=$!
exec 8>&-
start=$(date +%s)
status=exit
while kill -0 "$pid" 2>/dev/null; do
    if [ -d stop ]; then status=canceled; break; fi
    if [ "$(( $(date +%s) - start ))" -ge "$timeout" ]; then status=timeout; break; fi
    sleep 0.2
done
if [ "$status" != exit ]; then
    kill -TERM "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || :
    sleep 1
    kill -KILL "-$pid" 2>/dev/null || :
fi
code=0
wait "$pid" || code=$?
# Clear descendants that still hold the output stream after the shell exits.
kill -KILL "-$pid" 2>/dev/null || :
tries=0
while kill -0 "$reader" 2>/dev/null && [ "$tries" -lt 10 ]; do
    sleep 0.1
    tries=$((tries + 1))
done
kill -KILL "-$reader" 2>/dev/null || :
wait "$reader" || :
printf '%s %s\n' "$status" "$code" > result.tmp
mv result.tmp result
rm -f fifo
`

func (r *remoteCommand) launch(ctx context.Context, command string) error {
	body := `for utility in nohup setsid flock mkfifo head date; do command -v "$utility" >/dev/null; done
mkdir -p "$task"
if [ ! -f "$task/signature" ] && [ ! -f "$task/canceled-before-launch" ]; then
    nohup setsid /bin/sh -c ` + quotePOSIX(commandWorker) + ` sh "$task" "$signature" ` + quotePOSIX(r.directory) + " " + quotePOSIX(command) + " " + strconv.Itoa(commandSeconds(ctx)) + ` </dev/null >/dev/null 2>&1 &
fi
tries=0
while [ ! -f "$task/signature" ] && [ ! -f "$task/canceled-before-launch" ] && [ "$tries" -lt 50 ]; do
    sleep 0.1
    tries=$((tries + 1))
done
`
	_, err := r.run(ctx, r.script(body))
	if err != nil {
		return &CommandTransportError{err}
	}
	return nil
}

const verifyCommand = `if [ -f "$task/canceled-before-launch" ]; then
    if [ "$(cat "$task/canceled-before-launch")" = "$signature" ]; then printf 'canceled 0\n'; else printf 'conflict\n'; fi
    exit
fi
if [ ! -f "$task/signature" ]; then printf 'missing\n'; exit; fi
if [ "$(cat "$task/signature")" != "$signature" ]; then printf 'conflict\n'; exit; fi
`

func (r *remoteCommand) observe(ctx context.Context) (*backgroundtool.Outcome, error) {
	change, err := r.run(ctx, r.script(verifyCommand+`cd "$task"
if [ ! -f result ]; then
    if flock -n lease true; then
        if [ ! -f result ]; then printf 'lost\n'; exit; fi
    else
        printf 'running\n'; exit
    fi
fi
cat result
if [ -f output ]; then head -c 65537 output; fi
`))
	if err != nil {
		return nil, &CommandTransportError{err}
	}
	state, output, _ := strings.Cut(change, "\n")
	switch state {
	case "missing":
		return nil, backgroundtask.ErrNotFound
	case "conflict":
		return nil, errors.New("task ID is already bound to another command")
	case "running":
		return nil, nil
	case "lost":
		return nil, errors.New("remote command outcome is unknown")
	}
	status, rawCode, _ := strings.Cut(state, " ")
	code, err := strconv.Atoi(rawCode)
	if err != nil {
		return nil, fmt.Errorf("invalid remote command status %q", state)
	}
	switch status {
	case "canceled":
		return &backgroundtool.Outcome{Status: backgroundtask.StatusCanceled}, nil
	case "timeout", "exit":
		data, err := json.Marshal(filesystem.ExecuteResponse{
			ExitCode: &code, Output: strings.ToValidUTF8(output[:min(len(output), fileLimit)], "�"),
			Truncated: len(output) > fileLimit, TimedOut: status == "timeout",
		})
		return &backgroundtool.Outcome{Status: backgroundtask.StatusCompleted, Data: data}, err
	default:
		return nil, fmt.Errorf("invalid remote command status %q", state)
	}
}

func (r *remoteCommand) Wait(ctx context.Context) (*backgroundtool.Outcome, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		outcome, err := r.observe(ctx)
		if err != nil || outcome != nil {
			return outcome, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *remoteCommand) Stop(ctx context.Context) error {
	state, err := r.run(ctx, r.script(`mkdir -p "$task"
cd "$task"
exec 8>launch
flock 8
if [ -f canceled-before-launch ]; then
    if [ "$(cat canceled-before-launch)" != "$signature" ]; then printf 'conflict\n'; fi
    exit
fi
if [ ! -f signature ]; then
    printf '%s\n' "$signature" > canceled.tmp
    mv canceled.tmp canceled-before-launch
    exit
fi
if [ "$(cat signature)" != "$signature" ]; then printf 'conflict\n'; exit; fi
if [ ! -f result ]; then mkdir -p stop; fi
`))
	if err != nil {
		return err
	}
	switch strings.TrimSpace(state) {
	case "":
		return nil
	default:
		return errors.New("task ID is already bound to another command")
	}
}
