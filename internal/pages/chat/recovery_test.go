package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggdream/microlight/internal/localtools"
)

func TestContinueFailedReplyUsesRecordedToolResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var step atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatRequestMessage }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Error("invalid request")
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		last := request.Messages[len(request.Messages)-1].Content
		switch value := step.Add(1); value {
		case 1, 3:
			id := "write-original"
			args := `{"path":"out.txt","content":"deliverable"}`
			if value == 3 {
				if !strings.Contains(last, "Continue the original task") || len(request.Messages) < 2 || !strings.Contains(request.Messages[len(request.Messages)-2].Content, "out.txt") {
					t.Error("retry was missing the recovery journal")
				}
				id = "write-duplicate"
				args = `{"content":"deliverable","path":"out.txt"}`
			}
			startToolCall(w, id, localtools.Write, args)
		case 2:
			http.Error(w, "mock provider failure", http.StatusBadRequest)
		case 4:
			if !strings.Contains(last, "Result recorded before the interruption") {
				t.Errorf("continued model request did not use prior receipt: %q", last)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Finished remaining work\"}}]}\n\ndata: [DONE]\n\n")
		default:
			t.Error("unexpected additional request")
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{ctx: ctx, store: store, backend: newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test"})}
	chat := app.newConversation("deliverable", localtools.Full, dir)
	reply := &turn{message: message{Role: "assistant"}, State: "pending"}
	chat.Turns = []*turn{userTurn("Write the deliverable and then summarize it"), reply}
	updates := make(chan func(), 30)
	update := func(fn func()) { updates <- fn }
	run := func() {
		t.Helper()
		app.startReply(chat, reply, 1, "", app.backend, modelChoice{}, update, func() {})
		for chat.isRunning() {
			select {
			case fn := <-updates:
				fn()
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	run()
	if reply.State != "error" || len(reply.deliverables()) != 1 {
		t.Fatalf("expected a failed reply with one finished deliverable, got %s and %+v", reply.State, reply.Activity)
	}
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("changed by another app"), 0600); err != nil {
		t.Fatal(err)
	}
	offset := len(reply.Activity)
	reply.RecoveryOffset = &offset
	reply.State, reply.Error = "pending", ""
	run()
	if reply.State != "done" || reply.Content != "Finished remaining work" {
		t.Fatalf("unexpected continuation: %s %q", reply.State, reply.Content)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "changed by another app" {
		t.Fatalf("continuation repeated the completed write: %q, %v", data, err)
	}
}

func startToolCall(w http.ResponseWriter, id, name, arguments string) {
	call, _ := json.Marshal(map[string]any{
		"index": 0, "id": id, "type": "function",
		"function": map[string]string{"name": name, "arguments": arguments},
	})
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[%s]}}]}\n\ndata: [DONE]\n\n", call)
}
