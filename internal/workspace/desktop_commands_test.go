package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/cloudwego/eino/adk/filesystem"
)

func TestDesktopCommandReconnect(t *testing.T) {
	const token = "desktop-test-token-00000000000000"
	t.Setenv("MICROLIGHT_DESKTOP_TOKEN", token)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	root := t.TempDir()
	files, err := openDesktopFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "commands.sqlite3")
	store, err := newDesktopCommandStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	var dropFirst atomic.Bool
	handler := desktopHandler(&testDesktop{}, "desktop-session", token, files, store)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == desktopPath+"commands/start" && dropFirst.CompareAndSwap(false, true) {
			handler.ServeHTTP(httptest.NewRecorder(), r)
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				connection.Close()
			}
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	connect := func(origin string) backgroundshell.RecoverableShell {
		t.Helper()
		rt, err := Open(ctx, Target{Kind: "desktop", Resource: origin, Directory: "/"}, token)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rt.Close() })
		ops, err := DiscoverOperations(ctx, rt)
		if err != nil || !ops.Capabilities.RecoverableShell || ops.Commands == nil {
			t.Fatalf("missing command capability: %+v, %v", ops.Capabilities, err)
		}
		return ops.Commands
	}
	command := `test -z "${MICROLIGHT_DESKTOP_TOKEN+x}" || exit 99; printf x >> proof.txt; sleep 2; printf success; exit 7`
	delayed := "sleep 4; printf x > canceled.txt"
	child := `/bin/sh -c 'touch child-ready; sleep 2; touch child-survived' & wait`
	if runtime.GOOS == "windows" {
		command = `if (Test-Path Env:MICROLIGHT_DESKTOP_TOKEN) { exit 99 }; [IO.File]::AppendAllText((Join-Path (Get-Location) 'proof.txt'), 'x'); Start-Sleep 2; Write-Output success; exit 7`
		delayed = `Start-Sleep 4; Set-Content canceled.txt x`
		child = `$p = Start-Process powershell.exe -PassThru -ArgumentList '-NoProfile -Command "New-Item child-ready; Start-Sleep 2; New-Item child-survived"'; $p.WaitForExit()`
	}
	start := &backgroundshell.StartCommandRequest{TaskID: "launch", Command: command}
	request := &backgroundshell.RecoverCommandRequest{TaskID: start.TaskID, Command: command}
	var lost *CommandTransportError
	if _, err := connect(server.URL).StartCommand(ctx, start); !errors.As(err, &lost) {
		t.Fatalf("lost launch reply: %v", err)
	}
	reconnected := connect(server.URL)
	run, err := reconnected.RecoverCommand(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	observer, stopObserving := context.WithCancel(ctx)
	stopObserving()
	if _, err := run.Wait(observer); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled observer = %v", err)
	}
	run, err = reconnected.StartCommand(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := run.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var result filesystem.ExecuteResponse
	if err := json.Unmarshal(outcome.Data, &result); err != nil || result.ExitCode == nil || *result.ExitCode != 7 {
		t.Fatalf("command result = %+v, %v", result, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "proof.txt")); err != nil || string(data) != "x" {
		t.Fatalf("expected one launch, contents %q: %v", data, err)
	}
	reopened, err := newDesktopCommandStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.db.Close()
	restarted := httptest.NewServer(desktopHandler(&testDesktop{}, "new-session", token, files, reopened))
	defer restarted.Close()
	run, err = connect(restarted.URL).RecoverCommand(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := run.Wait(ctx)
	if err != nil || string(saved.Data) != string(outcome.Data) {
		t.Fatalf("restart recovery: %+v, %v", saved, err)
	}
	// An executor crash after claiming a launch leaves no trustworthy
	// result. Reopening the store must never run the task again.
	abandonedID := fmt.Sprintf("%x", sha256.Sum256([]byte(".\x00abandoned")))
	signature := fmt.Sprintf("%x", sha256.Sum256([]byte(command)))
	if _, err := store.db.Exec("INSERT INTO commands VALUES (?, ?, NULL)", abandonedID, signature); err != nil {
		t.Fatal(err)
	}
	if _, err := connect(restarted.URL).StartCommand(ctx, &backgroundshell.StartCommandRequest{TaskID: "abandoned", Command: command}); err == nil || !strings.Contains(err.Error(), "outcome is unknown") {
		t.Fatalf("abandoned launch: %v", err)
	}
	for _, id := range []string{"cancel-running", "cancel-before-start"} {
		start = &backgroundshell.StartCommandRequest{TaskID: id, Command: delayed}
		request = &backgroundshell.RecoverCommandRequest{TaskID: id, Command: delayed}
		if id == "cancel-running" {
			if _, err := reconnected.StartCommand(ctx, start); err != nil {
				t.Fatal(err)
			}
		}
		run, err := reconnected.(CommandStopper).StopCommand(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		outcome, err := run.Wait(ctx)
		if err != nil || outcome.Status != backgroundtask.StatusCanceled {
			t.Fatalf("cancel = %+v, %v", outcome, err)
		}
		run, err = reconnected.StartCommand(ctx, start)
		if err != nil {
			t.Fatal(err)
		}
		outcome, err = run.Wait(ctx)
		if err != nil || outcome.Status != backgroundtask.StatusCanceled {
			t.Fatalf("late start = %+v, %v", outcome, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "canceled.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled command performed side effect: %v", err)
	}
	if _, err := reconnected.StartCommand(ctx, &backgroundshell.StartCommandRequest{TaskID: "launch", Command: delayed}); err == nil {
		t.Fatal("task ID was rebound to another command")
	}
	run, err = reconnected.StartCommand(ctx, &backgroundshell.StartCommandRequest{TaskID: "cancel-child", Command: child})
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "child-ready")); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("child process never started")
		case <-time.After(25 * time.Millisecond):
		}
	}
	if err := run.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	outcome, err = run.Wait(ctx)
	if err != nil || outcome.Status != backgroundtask.StatusCanceled {
		t.Fatalf("cancel child = %+v, %v", outcome, err)
	}
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "child-survived")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("child survived cancellation: %v", err)
	}
}

type unavailableDesktop struct{ ComputerDriver }

func (unavailableDesktop) GetCapabilities(context.Context) (ComputerCapabilities, error) {
	return ComputerCapabilities{}, errors.New("interactive desktop unavailable")
}

func TestDesktopCommandRecoveryWithoutWorkspaceDirectory(t *testing.T) {
	const token = "desktop-test-token-00000000000000"
	root := t.TempDir()
	dir := filepath.Join(root, "work")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	files, err := openDesktopFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newDesktopCommandStore(filepath.Join(t.TempDir(), "commands.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	server := httptest.NewServer(desktopHandler(unavailableDesktop{}, "desktop-session", token, files, store))
	defer server.Close()
	target := Target{Kind: "desktop", Resource: server.URL, Directory: "/work"}
	ctx := t.Context()
	open := func() Runtime {
		t.Helper()
		rt, err := Open(ctx, target, token)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rt.Close() })
		return rt
	}
	const command = "echo original"
	start := &backgroundshell.StartCommandRequest{TaskID: "existing", Command: command}
	run, err := open().(CommandProvider).Commands().StartCommand(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := run.Wait(ctx)
	if err != nil || outcome.Status != backgroundtask.StatusCompleted {
		t.Fatalf("first outcome = %+v, %v", outcome, err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	rt := open()
	operations, err := DiscoverOperations(ctx, rt)
	if err != nil || operations.Files != nil || operations.Capabilities.Computer != (ComputerCapabilities{}) {
		t.Fatalf("operations after directory removal: %+v, %v", operations.Capabilities, err)
	}
	commands, err := DiscoverCommands(ctx, rt)
	if err != nil || commands == nil {
		t.Fatalf("command capability after directory removal: %v", err)
	}
	run, err = commands.RecoverCommand(ctx, &backgroundshell.RecoverCommandRequest{TaskID: start.TaskID, Command: command})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := run.Wait(ctx)
	if err != nil || string(saved.Data) != string(outcome.Data) {
		t.Fatalf("recovered outcome = %+v, %v", saved, err)
	}
	if _, err := commands.StartCommand(ctx, &backgroundshell.StartCommandRequest{TaskID: "new", Command: command}); err == nil {
		t.Fatal("new command started without its directory")
	}
}
