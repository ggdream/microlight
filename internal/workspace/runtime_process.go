package workspace

import (
	"bytes"
	"context"
	"errors"
	"io"

	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

const fileLimit = 64 << 10
const remoteResponseLimit = fileLimit + 128

type boundedOutput struct {
	mu        sync.Mutex
	data      bytes.Buffer
	limit     int
	truncated bool
}

func (o *boundedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := min(len(p), o.limit-o.data.Len())
	o.data.Write(p[:n])
	o.truncated = o.truncated || n < len(p)
	return len(p), nil
}

func commandContext(ctx context.Context, request *filesystem.ExecuteRequest) (context.Context, context.CancelFunc) {
	timeout := 2 * time.Minute
	if request.Timeout != nil {
		timeout = min(timeout, *request.Timeout)
	}
	return context.WithTimeout(ctx, timeout)
}

func runCLI(ctx context.Context, stdin io.Reader, name string, args ...string) (string, string, bool, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hideCommandWindow(cmd)
	cmd.Stdin = stdin
	cmd.WaitDelay = time.Second
	out, stderr := &boundedOutput{limit: remoteResponseLimit}, &boundedOutput{limit: fileLimit}
	cmd.Stdout, cmd.Stderr = out, stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.data.String(), stderr.data.String(), out.truncated, err
}

func shellResult(output string, truncated bool, err error) (*filesystem.ExecuteResponse, error) {
	code := 0
	if err != nil {
		var exit interface{ ExitStatus() int }
		var process *exec.ExitError
		switch {
		case errors.As(err, &exit):
			code = exit.ExitStatus()
		case errors.As(err, &process):
			code = process.ExitCode()
		default:
			return nil, err
		}
	}
	truncated = truncated || len(output) > fileLimit
	return &filesystem.ExecuteResponse{Output: strings.ToValidUTF8(output[:min(len(output), fileLimit)], "�"), ExitCode: &code, Truncated: truncated}, nil
}

func readText(r io.Reader, request *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	data, err := io.ReadAll(io.LimitReader(r, fileLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > fileLimit {
		return nil, errors.New("目标内容超过 64 KiB")
	}
	if !bytes.Equal(bytes.ToValidUTF8(data, nil), data) || bytes.ContainsRune(data, 0) {
		return nil, errors.New("目标不是 UTF-8 文本")
	}
	content := string(data)
	if request.Offset > 1 || request.Limit > 0 {
		lines := strings.SplitAfter(content, "\n")
		start := min(len(lines), max(request.Offset-1, 0))
		end := len(lines)
		if request.Limit > 0 {
			end = start + min(request.Limit, len(lines)-start)
		}
		content = strings.Join(lines[start:end], "")
	}
	return &filesystem.FileContent{Content: content}, nil
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
