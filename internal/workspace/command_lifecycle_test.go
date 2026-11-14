package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
	"uuid"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/cloudwego/eino/adk/filesystem"
)

func TestDockerCommandLifecycle(t *testing.T) {
	if os.Getenv("MICROLIGHT_TEST_DOCKER") != "1" {
		t.Skip("set MICROLIGHT_TEST_DOCKER to test with Docker")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	engine, _, err := ListContainers(ctx, "docker")
	if err != nil {
		t.Fatal(err)
	}
	target := Target{Kind: "container", Directory: "/workspace", Engine: engine}
	id := uuid.NewV7().String()
	t.Cleanup(func() { CleanupContainer(target, id, true) })
	target, err = CreateContainer(ctx, target, "postgres:16-alpine", "", id, ContainerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	connect := func() Runtime {
		t.Helper()
		rt, err := Open(ctx, target, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rt.Close() })
		return rt
	}
	testCommandLifecycle(t, ctx, connect)
}

func TestSSHCommandLifecycle(t *testing.T) {
	key := os.Getenv("MICROLIGHT_TEST_SSH_KEY")
	if key == "" {
		t.Skip("set MICROLIGHT_TEST_SSH_KEY to test with SSH")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	target := Target{
		Kind: "remote", Directory: os.Getenv("MICROLIGHT_TEST_SSH_DIR"),
		Host: "127.0.0.1", Port: "42173", User: "root", Auth: "key", KeyFile: key,
	}
	_, err := Open(ctx, target, "")
	var host *UnknownHostKey
	if !errors.As(err, &host) {
		t.Fatalf("expected host key prompt, got %v", err)
	}
	target.HostKey = host.Fingerprint
	connect := func() Runtime {
		t.Helper()
		rt, err := Open(ctx, target, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rt.Close() })
		return rt
	}
	testCommandLifecycle(t, ctx, connect)
	t.Run("MicroVMTaskIsolation", func(t *testing.T) {
		guest := target
		guest.Kind = "microvm"
		vm, err := Open(ctx, guest, "")
		if err != nil {
			t.Fatal(err)
		}
		defer vm.Close()
		vmCommands := vm.(CommandProvider).Commands()
		if vmCommands == nil {
			t.Fatal("MicroVM recoverable shell capability is missing")
		}
		id := uuid.NewV7().String()
		command := "printf vm"
		run, err := vmCommands.StartCommand(ctx, &backgroundshell.StartCommandRequest{TaskID: id, Command: command})
		if err != nil {
			t.Fatal(err)
		}
		sshCommands := connect().(CommandProvider).Commands()
		request := &backgroundshell.RecoverCommandRequest{TaskID: id, Command: command}
		if _, err := sshCommands.RecoverCommand(ctx, request); !errors.Is(err, backgroundtask.ErrNotFound) {
			t.Fatalf("ordinary SSH recovered a MicroVM task: %v", err)
		}
		stop, err := sshCommands.(CommandStopper).StopCommand(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if outcome, err := stop.Wait(ctx); err != nil || outcome.Status != backgroundtask.StatusCanceled {
			t.Fatalf("SSH cancellation did not stay in its own scope: %+v, %v", outcome, err)
		}
		if outcome, err := run.Wait(ctx); err != nil || outcome.Status != backgroundtask.StatusCompleted {
			t.Fatalf("MicroVM task affected by SSH cancellation: %+v, %v", outcome, err)
		}
	})
}

func testCommandLifecycle(t *testing.T, ctx context.Context, connect func() Runtime) {
	t.Helper()
	rt := connect()
	commands := rt.(CommandProvider).Commands()
	if commands == nil {
		t.Fatal("recoverable shell capability missing")
	}
	id := uuid.NewV7().String()
	start := &backgroundshell.StartCommandRequest{
		TaskID: id, Command: "printf x >> " + id + "; sleep 2; cat " + id + "; exit 7", Attempt: 1,
	}
	run, err := commands.StartCommand(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	expired, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := run.Wait(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled observation = %v", err)
	}
	_ = rt.Close()
	rt = connect()
	commands = rt.(CommandProvider).Commands()
	start.Attempt++
	if _, err := commands.StartCommand(ctx, start); err != nil {
		t.Fatalf("retry: %v", err)
	}
	request := &backgroundshell.RecoverCommandRequest{TaskID: id, Command: start.Command, Attempt: 2}
	if _, err := commands.RecoverCommand(ctx, &backgroundshell.RecoverCommandRequest{TaskID: id, Command: "true"}); err == nil {
		t.Fatal("accepted a different command for the same task ID")
	}
	run, err = commands.RecoverCommand(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := run.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var result filesystem.ExecuteResponse
	if err := json.Unmarshal(outcome.Data, &result); err != nil || outcome.Status != backgroundtask.StatusCompleted || result.ExitCode == nil || *result.ExitCode != 7 || result.Output != "x" {
		t.Fatalf("recovered result = %+v, %s, %v", result, outcome.Status, err)
	}
	id = uuid.NewV7().String()
	start = &backgroundshell.StartCommandRequest{TaskID: id, Command: "sleep 4 && touch " + id}
	run, err = commands.StartCommand(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := run.Stop(ctx); err != nil {
			t.Fatal(err)
		}
	}
	outcome, err = run.Wait(ctx)
	if err != nil || outcome.Status != backgroundtask.StatusCanceled {
		t.Fatalf("stop outcome = %+v, %v", outcome, err)
	}
	run, err = commands.StartCommand(ctx, &backgroundshell.StartCommandRequest{TaskID: uuid.NewV7().String(), Command: "head -c 100000 /dev/zero | tr '\\000' x"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = run.Wait(ctx)
	if err != nil || json.Unmarshal(outcome.Data, &result) != nil || !result.Truncated || len(result.Output) != fileLimit {
		t.Fatalf("expected truncated output, outcome = %.120s, error = %v", outcome.Data, err)
	}
	if path, _, err := rt.(ResolvedFileSystem).Resolve(ctx, id, false); err == nil {
		t.Fatalf("canceled command performed side effect at %s", path)
	}

	// A canceled launch may reach the executor after the cancellation.
	id = uuid.NewV7().String()
	beforeLaunch := &backgroundshell.RecoverCommandRequest{TaskID: id, Command: "touch " + id, Attempt: 1}
	if _, err := commands.RecoverCommand(ctx, beforeLaunch); !errors.Is(err, backgroundtask.ErrNotFound) {
		t.Fatalf("observe before launch = %v", err)
	}
	stopper := commands.(CommandStopper)
	run, err = stopper.StopCommand(ctx, beforeLaunch)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = run.Wait(ctx)
	if err != nil || outcome.Status != backgroundtask.StatusCanceled {
		t.Fatalf("cancel before launch = %+v, %v", outcome, err)
	}
	run, err = commands.StartCommand(ctx, &backgroundshell.StartCommandRequest{
		TaskID: id, Command: beforeLaunch.Command, Attempt: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = run.Wait(ctx)
	if err != nil || outcome.Status != backgroundtask.StatusCanceled {
		t.Fatalf("late launch = %+v, %v", outcome, err)
	}
	if _, _, err := rt.(ResolvedFileSystem).Resolve(ctx, id, false); err == nil {
		t.Fatal("late launch performed a side effect")
	}
	if _, err := stopper.StopCommand(ctx, &backgroundshell.RecoverCommandRequest{TaskID: id, Command: "true"}); err == nil {
		t.Fatal("cancellation accepted a different command for an existing ID")
	}
}
