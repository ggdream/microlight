package background

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"uuid"

	"github.com/cloudwego/eino/adk/backgroundtask"
	"github.com/cloudwego/eino/adk/backgroundtask/local"
	"github.com/ggdream/microlight/internal/shell"
)

type Tasks struct {
	ctx     context.Context
	manager *backgroundtask.Manager
	runner  *local.Runner
	mu      sync.RWMutex
	origins map[string]origin
	active  []string
}

type origin struct {
	conversation string
	directory    string
}

type Row struct {
	Conversation string
	Directory    string
	*backgroundtask.Task
}

func New(ctx context.Context) (*Tasks, error) {
	registry := backgroundtask.NewExecutorRegistry()
	manager, err := backgroundtask.New(ctx, &backgroundtask.Config{
		Executors: registry,
		IDGen: func(context.Context, *backgroundtask.AllocateTaskIDRequest) (string, error) {
			return uuid.NewV7().String(), nil
		},
	})
	if err != nil {
		return nil, err
	}
	runner, err := local.New(&local.Config{Manager: manager, Executors: registry})
	if err != nil {
		_ = manager.Close(ctx)
		return nil, err
	}
	return &Tasks{ctx: ctx, manager: manager, runner: runner, origins: make(map[string]origin)}, nil
}

func (t *Tasks) Close(ctx context.Context) error { return t.manager.Close(ctx) }

func (t *Tasks) Start(ctx context.Context, owner, script, directory string) (string, error) {
	task, err := t.runner.Run(ctx, &local.Input{
		Kind: "shell", Description: script, RunInBackground: true,
	}, func(ctx context.Context, _ backgroundtask.ExecutionRuntime) (string, error) {
		response, err := shell.Execute(ctx, script, directory, nil)
		if err != nil {
			return "", err
		}
		if response.TimedOut {
			return "", fmt.Errorf("command timed out\n%s", response.Output)
		}
		if response.ExitCode != nil && *response.ExitCode != 0 {
			return "", fmt.Errorf("exit %d\n%s", *response.ExitCode, response.Output)
		}
		return response.Output, nil
	})
	if err != nil {
		return "", err
	}
	id := task.Spec.ID
	t.mu.Lock()
	t.origins[id] = origin{owner, directory}
	t.active = append(t.active, id)
	t.mu.Unlock()
	return id, nil
}

func (t *Tasks) ActiveRows() []Row {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	rows := make([]Row, 0, len(t.active))
	t.active = slices.DeleteFunc(t.active, func(id string) bool {
		task, err := t.manager.Get(t.ctx, id)
		if err != nil {
			return false
		}
		if task.DoneAt != nil {
			return true
		}
		source := t.origins[id]
		rows = append(rows, Row{Conversation: source.conversation, Directory: source.directory, Task: task})
		return false
	})
	slices.Reverse(rows)
	return rows
}

func (t *Tasks) Stop(ctx context.Context, id string) error {
	_, err := t.manager.RequestCancel(ctx, id)
	if errors.Is(err, backgroundtask.ErrAlreadyTerminal) {
		return nil
	}
	return err
}

func (t *Tasks) Status(ctx context.Context, owner, id string) (string, error) {
	t.mu.RLock()
	taskOwner := t.origins[id].conversation
	t.mu.RUnlock()
	if taskOwner == "" || taskOwner != owner {
		return "", errors.New("background task was not found in this conversation")
	}
	task, err := t.manager.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\n%s%s", task.Status, task.ResultData, task.ResultError), nil
}
