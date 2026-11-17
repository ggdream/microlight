package background

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk/backgroundtask"
)

func TestShellTaskOutlivesReplyAndCanBeStopped(t *testing.T) {
	tasks, err := New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := tasks.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	workspace := t.TempDir()
	print, wait := "printf output-marker", "sleep 30"
	if runtime.GOOS == "windows" {
		print, wait = "Write-Output output-marker", "Start-Sleep -Seconds 30"
	}
	reply, endReply := context.WithCancel(t.Context())
	defer endReply()
	id, err := tasks.Start(reply, "chat-1", print, workspace)
	if err != nil {
		t.Fatal(err)
	}
	endReply()
	waitForTaskStatus(t, tasks, id, backgroundtask.StatusCompleted)
	output, err := tasks.Status(t.Context(), "chat-1", id)
	if err != nil || !strings.Contains(output, "output-marker") {
		t.Fatalf("missing shell result: %q, %v", output, err)
	}
	if _, err := tasks.Status(t.Context(), "chat-2", id); err == nil {
		t.Fatal("another conversation read the task output")
	}

	id, err = tasks.Start(t.Context(), "chat-1", wait, workspace)
	if err != nil {
		t.Fatal(err)
	}
	waitForTaskStatus(t, tasks, id, backgroundtask.StatusRunning)
	if rows := tasks.ActiveRows(); len(rows) != 1 || rows[0].Spec.ID != id {
		t.Fatalf("active list contains unexpected tasks: %+v", rows)
	}
	stop, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := tasks.Stop(stop, id); err != nil {
		t.Fatal(err)
	}
	waitForTaskStatus(t, tasks, id, backgroundtask.StatusCanceled)
	if rows := tasks.ActiveRows(); len(rows) != 0 {
		t.Fatalf("terminal tasks remained in the active list: %+v", rows)
	}
}

func waitForTaskStatus(t *testing.T, tasks *Tasks, id string, expected backgroundtask.Status) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		task, err := tasks.manager.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == expected {
			return
		}
		if task.DoneAt != nil {
			t.Fatalf("unexpected task state %s (%s), wanted %s", task.Status, task.ResultError, expected)
		}
		_, err = tasks.manager.WaitForTaskVersion(ctx, &backgroundtask.WaitForTaskVersionRequest{TaskID: id, AfterVersion: task.Version})
		if err != nil {
			t.Fatalf("waiting for %s from %s: %v", expected, task.Status, err)
		}
	}
}
