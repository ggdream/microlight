package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/workspace"
)

func TestRemoteCommandJournalSurvivesReopen(t *testing.T) {
	target := workspace.Target{Kind: "remote", Directory: "/work"}
	testRemoteCommandReopen(t, target)
}

func TestDesktopCommandRecoveryChecksPersistedExecutor(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/desktop/info":
			_, _ = w.Write([]byte(`{"SessionID":"desktop","CommandStoreID":"replacement","Files":true,"Commands":true}`))
		case "/v1/desktop/files/resolve":
			_, _ = w.Write([]byte(`{"Path":"/","IsDir":true}`))
		default:
			requests.Add(1)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := workspace.Target{Kind: "desktop", Directory: "/", Resource: server.URL}
	chat := &conversation{ID: newID(), target: target, Turns: []*turn{
		userTurn("Run a command"), {message: message{Role: "assistant"}, State: "pending"},
	}}
	if err := store.saveHistory(history{Chats: []*conversation{chat}, Selected: chat.ID}, chat); err != nil {
		t.Fatal(err)
	}
	record := localtools.RemoteCommand{ID: newID(), CallID: "tool", Command: "echo original", ExecutorID: "journal/original"}
	journal := store.commandJournal(chat.ID, chat.branchID, target.Identity(), 1)
	if err := journal.Record(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	conflict := record
	conflict.ExecutorID = "journal/replacement"
	if err := journal.Record(t.Context(), conflict); err == nil {
		t.Fatal("allowed a different executor to claim the recorded command")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, err := reopened.loadRemoteCommands(chat.ID, chat.branchID, 1)
	if err != nil || len(saved) != 1 || saved[0].RemoteCommand != record {
		t.Fatalf("restored record = %+v, %v", saved, err)
	}
	connect := func(ctx context.Context) (workspace.Runtime, error) {
		return workspace.Open(ctx, target, "desktop-test-token-00000000000000")
	}
	for _, stop := range []bool{false, true} {
		if _, err := localtools.RecoverRemoteCommand(t.Context(), connect, saved[0].RemoteCommand, stop); err == nil || !strings.Contains(err.Error(), "command journal changed") {
			t.Fatalf("recovery with stop=%t: %v", stop, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("sent %d command requests to the replacement executor", requests.Load())
	}
}

func TestRemoteCommandsFollowBranchAncestry(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	chat := &conversation{ID: newID(), target: workspace.Target{Kind: "remote", Directory: "/work"}, Turns: []*turn{
		userTurn("First"), {message: message{Role: "assistant"}, State: "pending"},
		userTurn("Second"), {message: message{Role: "assistant"}, State: "pending"},
	}}
	h := history{Chats: []*conversation{chat}, Selected: chat.ID}
	if err := store.saveHistory(h, chat); err != nil {
		t.Fatal(err)
	}
	record := func(branch string, ordinal int, id string) {
		t.Helper()
		journal := store.commandJournal(chat.ID, branch, chat.target.Identity(), ordinal)
		if err := journal.Record(t.Context(), localtools.RemoteCommand{ID: id, CallID: "reused-id", Command: "true"}); err != nil {
			t.Fatal(err)
		}
	}
	record("", 1, "inherited")
	record("", 3, "detached")
	edited := *chat
	edited.branchID = newID()
	if err := store.saveForkedHistory(chat.ID, &edited, messageBranch{id: chat.ID, turns: chat.Turns}, 3); err != nil {
		t.Fatal(err)
	}
	record(edited.branchID, 3, "child-detached")
	grandchild := edited
	grandchild.branchID = newID()
	if err := store.saveForkedHistory(chat.ID, &grandchild, messageBranch{id: edited.branchID, turns: edited.Turns}, 3); err != nil {
		t.Fatal(err)
	}
	record(grandchild.branchID, 3, "current")
	app := &chatApp{store: store}
	app.loadHistory()
	restored := app.history.Chats[0]
	if commands := restored.Turns[1].remoteCommands; len(commands) != 1 || commands[0].ID != "inherited" {
		t.Fatalf("inherited commands = %+v", commands)
	}
	if commands := restored.Turns[3].remoteCommands; len(commands) != 1 || commands[0].ID != "current" {
		t.Fatalf("current commands = %+v", commands)
	}
	detached := restored.detachedCommands
	if detached == nil || len(detached.remoteCommands) != 2 || len(detached.Activity) != 2 ||
		detached.remoteCommands[0].ID != "detached" || detached.remoteCommands[1].ID != "child-detached" {
		t.Fatalf("older branch commands = %+v", detached)
	}
	if err := store.finishRemoteCommand(t.Context(), "detached", &backgroundtool.Outcome{Status: backgroundtask.StatusCanceled}); err != nil {
		t.Fatal(err)
	}
	app.loadHistory()
	detached = app.history.Chats[0].detachedCommands
	if detached == nil || len(detached.Activity) != 2 || detached.Activity[0].Pending || detached.Activity[0].Output == nil {
		t.Fatalf("finished command from original branch was not restored: %+v", detached)
	}
}

func TestDockerChatCommandReattachesAfterReopen(t *testing.T) {
	if os.Getenv("MICROLIGHT_TEST_DOCKER") != "1" {
		t.Skip("set MICROLIGHT_TEST_DOCKER to test with Docker")
	}
	engine, _, err := workspace.ListContainers(t.Context(), "docker")
	if err != nil {
		t.Fatal(err)
	}
	id := newID()
	target := workspace.Target{Kind: "container", Directory: "/workspace", Engine: engine}
	t.Cleanup(func() { workspace.CleanupContainer(target, id, true) })
	target, err = workspace.CreateContainer(t.Context(), target, "postgres:16-alpine", "", id, workspace.ContainerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	testRemoteCommandReopen(t, target)
}

func testRemoteCommandReopen(t *testing.T, target workspace.Target) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	chat := &conversation{ID: newID(), branchID: newID(), target: target, Turns: []*turn{
		userTurn("Run a remote command"), {message: message{Role: "assistant"}, State: "pending"},
	}}
	record := localtools.RemoteCommand{ID: newID(), CallID: "original-tool-call", Command: "printf once >> marker; sleep 2; cat marker"}
	journal := store.commandJournal(chat.ID, chat.branchID, target.Identity(), 1)
	if err := journal.Record(ctx, record); err == nil {
		t.Fatal("recorded a command before its turn was saved")
	}
	if err := store.saveHistory(history{Chats: []*conversation{chat}, Selected: chat.ID}, chat); err != nil {
		t.Fatal(err)
	}
	if err := journal.Record(ctx, record); err != nil {
		t.Fatal(err)
	}
	conflict := record
	conflict.Command = "different command"
	if err := journal.Record(ctx, conflict); err == nil {
		t.Fatal("allowed a different command with a reused ID")
	}
	connect := func(c context.Context) (workspace.Runtime, error) { return workspace.Open(c, target, "") }
	if target.Kind == "container" {
		rt, err := connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		cancelRecord := localtools.RemoteCommand{ID: newID(), CallID: "second-tool-call", Command: "sleep 30; touch canceled-marker"}
		if err := journal.Record(ctx, cancelRecord); err != nil {
			t.Fatal(err)
		}
		rt, err = connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = rt.(workspace.CommandProvider).Commands().StartCommand(ctx, &backgroundshell.StartCommandRequest{
			TaskID: cancelRecord.ID, Command: cancelRecord.Command, Attempt: 1,
		})
		rt.Close()
		if err != nil {
			t.Fatal(err)
		}
		_, err = rt.(workspace.CommandProvider).Commands().StartCommand(ctx, &backgroundshell.StartCommandRequest{
			TaskID: record.ID, Command: record.Command, Attempt: 1,
		})
		rt.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store = reopened
	app := &chatApp{store: store}
	app.loadHistory()
	entry := app.history.Chats[0].Turns[1]
	if entry.State != "stopped" || !entry.hasUnresolvedRemoteCommands() || len(entry.Activity) == 0 || !entry.Activity[0].Pending {
		t.Fatalf("restored turn = %+v", entry)
	}
	if entry.remoteCommands[0].RemoteCommand != record {
		t.Fatal("wrong command record restored")
	}
	var data []byte
	if target.Kind == "container" {
		outcome, err := localtools.RecoverRemoteCommand(ctx, connect, entry.remoteCommands[0].RemoteCommand, false)
		if err != nil || outcome.Status != backgroundtask.StatusCompleted {
			t.Fatalf("recover after reopening: %+v, %v", outcome, err)
		}
		if err := store.finishRemoteCommand(ctx, record.ID, outcome); err != nil {
			t.Fatal(err)
		}
		data = outcome.Data
		if len(entry.remoteCommands) != 2 {
			t.Fatal("cancelable command was not restored")
		}
		pending := entry.remoteCommands[1]
		stopped, err := localtools.RecoverRemoteCommand(ctx, connect, pending.RemoteCommand, true)
		if err != nil || stopped.Status != backgroundtask.StatusCanceled {
			t.Fatalf("cancel after reopening: %+v, %v", stopped, err)
		}
		if err := store.finishRemoteCommand(ctx, pending.ID, stopped); err != nil {
			t.Fatal(err)
		}
	} else {
		data, _ = json.Marshal(filesystem.ExecuteResponse{Output: "once"})
		if err := store.finishRemoteCommand(ctx, record.ID, &backgroundtool.Outcome{Status: backgroundtask.StatusCompleted, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	if len(data) == 0 {
		t.Fatal("empty recovered result")
	}
	app.loadHistory()
	entry = app.history.Chats[0].Turns[1]
	if entry.hasUnresolvedRemoteCommands() || entry.Activity[0].Pending || entry.Activity[0].Output == nil || *entry.Activity[0].Output != "once" {
		t.Fatalf("restored result = %+v", entry.Activity)
	}
}
