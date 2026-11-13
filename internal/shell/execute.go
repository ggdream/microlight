package shell

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

const maxOutput = 64 << 10

func Execute(ctx context.Context, command, dir string, requestedTimeout *time.Duration) (*filesystem.ExecuteResponse, error) {
	timeout := 2 * time.Minute
	if requestedTimeout != nil {
		timeout = min(timeout, *requestedTimeout)
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := platformShellCommand(commandCtx, command)
	cmd.Dir = dir
	cmd.Env = shellEnvironment(cmd.Environ())
	cmd.WaitDelay = time.Second
	output := &shellOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	result := &filesystem.ExecuteResponse{Output: strings.ToValidUTF8(output.buffer.String(), "�"), Truncated: output.truncated}
	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(commandCtx.Err(), context.DeadlineExceeded):
		result.TimedOut = true
	case err != nil:
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, err
		}
		code := exit.ExitCode()
		result.ExitCode = &code
	default:
		code := 0
		result.ExitCode = &code
	}
	return result, nil
}

func shellEnvironment(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, "BASH_ENV=") || strings.HasPrefix(item, "ENV=") || strings.HasPrefix(item, "BASH_FUNC_") {
			continue
		}
		out = append(out, item)
	}
	if runtime.GOOS == "darwin" {
		// Finder launches apps with a minimal PATH. Include standard locations
		// for Homebrew and Docker CLI installations without sourcing profiles.
		dirs := []string{os.Getenv("PATH"), "/opt/homebrew/bin", "/usr/local/bin"}
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, ".docker", "bin"))
		}
		if dirs[0] == "" {
			dirs = append(dirs[1:], "/usr/bin", "/bin", "/usr/sbin", "/sbin")
		}
		out = append(out, "PATH="+strings.Join(dirs, string(os.PathListSeparator)))
	}
	return out
}

type shellOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (out *shellOutput) Write(data []byte) (int, error) {
	available := maxOutput - out.buffer.Len()
	if len(data) > available {
		out.truncated = true
	}
	out.buffer.Write(data[:min(len(data), available)])
	return len(data), nil
}
