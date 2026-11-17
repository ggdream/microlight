package localtools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/ggdream/microlight/internal/background"
)

const ExecuteBackground = "execute_background"
const PublishArtifact = "publish_artifact"
const BackgroundStatus = "background_task_status"

type backgroundKey struct{}
type backgroundContext struct {
	tasks *background.Tasks
	owner string
}

func WithBackground(ctx context.Context, tasks *background.Tasks, owner string) context.Context {
	return context.WithValue(ctx, backgroundKey{}, backgroundContext{tasks, owner})
}

type BackgroundStatusTool struct{}

func (BackgroundStatusTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: BackgroundStatus, Desc: "Check a background task started in this conversation and read any final output.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task_id": {Type: schema.String, Required: true},
		}),
	}, nil
}

func (BackgroundStatusTool) InvokableRun(ctx context.Context, raw string, _ ...tool.Option) (string, error) {
	var args struct {
		ID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return "Invalid task ID", nil
	}
	run, _ := ctx.Value(backgroundKey{}).(backgroundContext)
	if run.tasks == nil {
		return "Background tasks are unavailable", nil
	}
	result, err := run.tasks.Status(ctx, run.owner, args.ID)
	if err != nil {
		return err.Error(), nil
	}
	return result, nil
}

func (op localOperation) startBackground(ctx context.Context) (string, error) {
	run, _ := ctx.Value(backgroundKey{}).(backgroundContext)
	if run.tasks == nil {
		return "", errors.New("background tasks are unavailable")
	}
	id, err := run.tasks.Start(ctx, run.owner, op.content, op.target)
	if err != nil {
		return "", err
	}
	return "Background task started. Task ID: " + id, nil
}
