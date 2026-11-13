package localtools

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/ggdream/microlight/internal/shell"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	filesystemmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
)

const Execute = filesystemmw.ToolNameExecute

type localCommandShell struct{}

func ShellMiddleware(ctx context.Context) (adk.ChatModelAgentMiddleware, error) {
	shellName := "Bash"
	if runtime.GOOS == "windows" {
		shellName = "PowerShell"
	}
	description := fmt.Sprintf("Run a non-interactive %s command on the user's computer. The working directory starts at the conversation workspace, or the user's home if no workspace is set. Shell state does not persist between calls. Command output is limited to 64 KiB and execution to two minutes.", shellName)
	return filesystemmw.New(ctx, &filesystemmw.MiddlewareConfig{
		Shell:             localCommandShell{},
		ExecuteToolConfig: &filesystemmw.ExecuteToolConfig{Desc: &description},
	})
}

func (localCommandShell) Execute(ctx context.Context, input *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	op, ok := ctx.Value(approvedOperationKey{}).(localOperation)
	if !ok || op.name != Execute || input == nil || input.Command != op.content {
		return nil, errors.New("shell command was not authorized")
	}
	return shell.Execute(ctx, op.content, op.target, input.Timeout)
}
