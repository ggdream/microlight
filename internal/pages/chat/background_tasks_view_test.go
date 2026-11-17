package chat

import (
	"testing"

	"github.com/cloudwego/eino/adk/backgroundtask"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/background"
)

func TestBackgroundTaskIndicatorHidesWhenIdle(t *testing.T) {
	const command = "test command"
	app := &chatApp{}
	view := func(c *ui.Context) {
		app.backgroundTaskIndicator(c)
	}
	idle := ui.NewTester(view, 500, 300)
	if err := idle.Click(locales.BackgroundTasks()); err == nil {
		t.Fatal("found an indicator when no background task was active")
	}
	app.backgroundRows = []background.Row{{Task: &backgroundtask.Task{
		Spec:   backgroundtask.Spec{ID: "test-1", Description: command},
		Status: backgroundtask.StatusRunning,
	}}}
	running := ui.NewTester(view, 500, 300)
	if err := running.Click(locales.BackgroundTasks()); err != nil {
		t.Fatal(err)
	}
	if !running.HasText(command) || !running.HasText(locales.TaskRunning()) {
		t.Fatalf("active command is missing from its popover: %v", running.Texts())
	}
}
