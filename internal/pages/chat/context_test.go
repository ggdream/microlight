package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
)

func TestRunnerImportsCompactsAndReplaysContext(t *testing.T) {
	const summary = "Project Lighthouse, use the original constraints."
	var summarizeCount atomic.Int32
	compressionVisible := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream   bool
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, _ := json.Marshal(request.Messages)
		history := string(data)
		if !request.Stream {
			summarizeCount.Add(1)
			select {
			case <-compressionVisible:
			case <-time.After(2 * time.Second):
				t.Error("compression start did not reach the caller while the summary request was in progress")
			}
			if !strings.Contains(history, "Legacy constraints") || strings.Contains(history, "Next action") {
				t.Error("summary should contain the legacy round and exclude the current user")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, summary)
			return
		}
		last := request.Messages[len(request.Messages)-1].Content
		if strings.Contains(last, "Branch") {
			if strings.Contains(history, summary) || strings.Contains(history, "Next action") {
				t.Error("branch received messages from the original session")
			}
		} else if !strings.Contains(history, summary) || strings.Contains(history, "Legacy constraints") {
			t.Error("model did not get the compacted session")
		}
		writeTestStream(w, "Lighthouse")
	}))
	defer server.Close()

	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	config := modelSettings{BaseURL: server.URL, Model: "test", ContextThreshold: 2048}
	var updates []compressionUpdate
	emit := func(delta replyDelta) error {
		if delta.compression != 0 {
			updates = append(updates, delta.compression)
		}
		if delta.compression == compressionStarted {
			compressionVisible <- struct{}{}
		}
		if delta.text != "" && !slices.Equal(updates, []compressionUpdate{compressionStarted, compressionFinished}) {
			t.Errorf("unexpected compression updates before reply: %v", updates)
		}
		return nil
	}
	legacy := userTurn("Legacy constraints: " + strings.Repeat("details ", 1000))
	input := []*turn{legacy, {message: message{Role: "assistant", Content: "Old reply"}}, userTurn("Next action")}
	if _, err := newChat(config).runSession(t.Context(), store, "original", input, nil, "", emit); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadEvents(t.Context(), "original", &adk.LoadSessionEventsRequest{Kinds: []adk.SessionEventKind{adk.SessionEventMessagesReplaced}})
	if err != nil || len(events.Events) != 1 || events.Events[0].MessagesReplaced == nil {
		t.Fatalf("replacement missing from SQLite: %v, %v", events, err)
	}
	store.Close()
	store, err = openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := newChat(config)
	emit = func(delta replyDelta) error {
		if delta.compression != 0 {
			t.Errorf("unexpected compression update on a short follow-up or branch: %v", delta.compression)
		}
		return nil
	}
	if _, err := backend.runSession(t.Context(), store, "original", []*turn{userTurn("Follow-up")}, nil, "", emit); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.runSession(t.Context(), store, "edited", []*turn{userTurn("Branch edited first prompt")}, nil, "", emit); err != nil {
		t.Fatal(err)
	}
	if summarizeCount.Load() != 1 {
		t.Fatalf("expected one summary across both sessions, got %d", summarizeCount.Load())
	}
}
