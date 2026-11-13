package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ggdream/microlight/internal/localtools"
)

func TestSessionPermissionChangesBetweenToolCalls(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "first.txt"), []byte("old header\nobsolete\n"), 0600); err != nil {
		t.Fatal(err)
	}
	access := localtools.NewAccess(workspace, localtools.Ask)
	access.SetMode(localtools.Full)
	workspaceInstruction := fmt.Sprintf("The workspace path is %q.", workspace)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []chatRequestMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Errorf("invalid request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !strings.Contains(request.Messages[0].Content, workspaceInstruction) {
			t.Error("session workspace missing from the agent instruction")
		}
		target := "first.txt"
		if request.Messages[len(request.Messages)-1].Role == "tool" {
			// The first tool call has finished and the same Runner is still executing.
			access.SetMode(localtools.Ask)
			target = "second.txt"
		}
		args, _ := json.Marshal(map[string]string{"path": target, "content": "test"})
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": target, "type": "function",
				"function": map[string]string{"name": localtools.Write, "arguments": string(args)},
			}}},
		}}})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Preparing %s\",\"reasoning_content\":\"Internal sample\"}}]}\n\n", target)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()

	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := localtools.WithAccess(t.Context(), access)
	config := modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test", Workspace: t.TempDir(), AccessMode: localtools.Full}
	var reply turn
	pending, err := newChat(config).runSession(ctx, store, "live-mode", []*turn{userTurn("Create two files")}, nil, "", func(delta replyDelta) error {
		reply.appendDelta(delta)
		return nil
	})
	if err != nil || pending == nil || pending.Approval == nil || pending.Approval.Target != filepath.Join(workspace, "second.txt") {
		t.Fatalf("second write should require approval: %+v, %v", pending, err)
	}
	if reply.Content != "" || reply.processPreview != "" || len(reply.Activity) != 4 ||
		reply.Activity[0].Text != "Preparing first.txt" || reply.Activity[1].Target != "first.txt" ||
		reply.Activity[2].Text != "Preparing second.txt" || reply.Activity[3].Target != "second.txt" {
		t.Fatalf("tool calls and model text were not routed to the timeline in order: %+v", reply.Activity)
	}
	first, second := reply.Activity[1], reply.Activity[3]
	if first.Pending || !second.Pending {
		t.Fatal("only the tool waiting for approval should remain pending")
	}
	if first.Arguments != `{"content":"test","path":"first.txt"}` || first.Diff == nil ||
		first.Diff.Added != 1 || first.Diff.Removed != 2 || len(first.Diff.Rows) != 3 ||
		second.Arguments != `{"content":"test","path":"second.txt"}` || second.Diff != nil {
		t.Fatalf("expected a diff only for the successful write: %+v, %+v", first, second)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "first.txt")); err != nil || string(data) != "test" {
		t.Fatalf("first write should succeed inside the session workspace: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "second.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second write must wait for approval: %v", err)
	}
}

func TestLocalToolApprovalIsScopedAndSurvivesRestart(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(workspace, "first.txt"), filepath.Join(workspace, "second.txt")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Role string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Messages[len(request.Messages)-1].Role == "tool" {
			writeTestStream(w, "Finished")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var calls []map[string]any
		for i, target := range []string{first, second} {
			args, _ := json.Marshal(map[string]string{"path": target, "content": "approved text"})
			calls = append(calls, map[string]any{
				"index": i, "id": fmt.Sprintf("write_%d", i), "type": "function",
				"function": map[string]string{"name": localtools.Write, "arguments": string(args)},
			})
		}
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": calls},
		}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()

	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	config := modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test", Workspace: workspace, AccessMode: localtools.Ask}
	emit := func(replyDelta) error { return nil }
	ctx := t.Context()
	const id = "approval"
	pending, err := newChat(config).runSession(ctx, store, id, []*turn{userTurn("Write the files")}, nil, "", emit)
	if err != nil || pending == nil || pending.Approval == nil || pending.Approval.Preview != "approved text" {
		t.Fatalf("expected an approval prompt: %+v, %v", pending, err)
	}
	absent := func(path string) {
		t.Helper()
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was touched before approval: %v", path, err)
		}
	}
	absent(first)
	absent(second)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	approved := pending.Approval.Target
	other := first
	if approved == first {
		other = second
	}
	remaining, err := newChat(config).runSession(ctx, store, id, nil, pending, localtools.AllowOnce, emit)
	if err != nil || remaining == nil || remaining.Approval == nil || remaining.Approval.Target != other {
		t.Fatalf("second operation must require its own approval: %+v, %v", remaining, err)
	}
	if data, err := os.ReadFile(approved); err != nil || string(data) != "approved text" {
		t.Fatalf("approved write failed: %q, %v", data, err)
	}
	absent(other)
	remaining, err = newChat(config).runSession(ctx, store, id, nil, remaining, localtools.Deny, emit)
	if err != nil || remaining != nil {
		t.Fatalf("denial should return to the agent: %+v, %v", remaining, err)
	}
	absent(other)
}
