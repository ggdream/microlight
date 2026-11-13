package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/session"
	"github.com/cloudwego/eino/schema"
	"github.com/egoist/mygo/ui"
)

func TestSQLiteSessionStoreConformance(t *testing.T) {
	session.RunConformanceTests(t, func(t testing.TB) adk.SessionEventStore[*schema.AgenticMessage] {
		store, err := openLocalStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
		return store
	}, schema.UserAgenticMessage)
}

func TestRunnerClarificationSurvivesRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []chatRequestMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		last := request.Messages[len(request.Messages)-1]
		switch last.Role {
		case "user":
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"Need destination\"}}]}\n\n")
			args := `{"question":"Which destination?","options":["A","B"]}`
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"ask_user\",\"arguments\":%q}}]}}]}\n\n", args)
		case "tool":
			call := request.Messages[len(request.Messages)-2]
			if last.Content != "Custom destination" || last.ToolCallID != "call_1" || call.Reasoning != "Need destination" {
				t.Errorf("incorrect clarification messages: %+v", request.Messages)
			}
			writeTestStream(w, "Travel plan")
			return
		default:
			t.Errorf("unexpected last role: %q", last.Role)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	ctx := t.Context()
	config := modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test"}
	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	const id = "clarification"
	backend := newChat(config)
	question, err := backend.runSession(ctx, store, id, []*turn{userTurn("Plan a trip")}, nil, "", func(replyDelta) error { return nil })
	if err != nil || question == nil || question.CheckpointID == "" || len(question.Options) != 2 {
		t.Fatalf("expected a checkpointed clarification, got %+v, %v", question, err)
	}
	requireV7 := func(id string) {
		t.Helper()
		u, err := uuid.Parse(id)
		if err != nil || u[6]>>4 != 7 {
			t.Fatalf("expected UUIDv7, got %q", id)
		}
	}
	requireV7(question.CheckpointID)
	var checkpointSession string
	if err := store.QueryRow("SELECT session_id FROM adk_checkpoints WHERE id = ?", question.CheckpointID).Scan(&checkpointSession); err != nil || checkpointSession != id {
		t.Fatalf("checkpoint must retain its session owner: %q, %v", checkpointSession, err)
	}
	events, err := store.LoadEvents(ctx, id, nil)
	if err != nil || len(events.Events) == 0 {
		t.Fatalf("expected stored session events: %v", err)
	}
	for _, event := range events.Events {
		requireV7(event.EventID)
	}
	chat := &conversation{ID: id, sessionID: id, Turns: []*turn{userTurn("Plan a trip"), {State: "waiting", message: message{Role: "assistant"}, Clarifications: []clarification{*question}}}}
	if err := store.saveHistory(history{Selected: id, Chats: []*conversation{chat}}, chat); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h, err := store.loadHistory()
	if err != nil || h.Chats[0].waitingReply() == nil {
		t.Fatalf("waiting reply was not restored: %v", err)
	}
	restored := h.Chats[0]
	pending := restored.waitingReply().Clarifications[0]
	// Reopen the saved question through the actual MyGo form and use its
	// resolved input value for the same Eino checkpoint.
	reply := restored.waitingReply()
	app := &chatApp{}
	window := ui.NewTester(func(c *ui.Context) { app.clarificationCards(c, restored, reply) }, 620, 320)
	if err := window.Click(locales.YourAnswer()); err != nil {
		t.Fatal(err)
	}
	window.Type("Custom destination")
	answer, ok := reply.clarificationSurface.Data().Get("/answer")
	if !ok || answer != "Custom destination" {
		t.Fatalf("the A2UI field did not update its binding: %v", answer)
	}
	var body strings.Builder
	question, err = newChat(config).runSession(ctx, store, restored.sessionID, nil, &pending, answer.(string), func(delta replyDelta) error {
		body.WriteString(delta.text)
		return nil
	})
	if err != nil || question != nil || body.String() != "Travel plan" {
		t.Fatalf("resume failed: %q, %+v, %v", body.String(), question, err)
	}
	_, found, err := store.Get(context.Background(), pending.CheckpointID)
	if found || err != nil {
		t.Fatalf("completed checkpoint was not removed: %v, %v", found, err)
	}
}

func userTurn(content string) *turn {
	return &turn{message: message{Role: "user", Content: content}}
}

func writeTestStream(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q}}]}\n\ndata: [DONE]\n\n", body)
}

func TestClarificationRemovesOptionLabels(t *testing.T) {
	got, err := parseClarification(`{"question":"第二轮怎么办？请选 1 或 2：","options":["1. 发新代号","（2）沿用旧代号","B、自己填写","1.5 倍速","1/2 份"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Question != "第二轮怎么办？" {
		t.Errorf("question = %q", got.Question)
	}
	want := []string{"发新代号", "沿用旧代号", "自己填写", "1.5 倍速", "1/2 份"}
	if !slices.Equal(got.Options, want) {
		t.Errorf("options = %q, want %q", got.Options, want)
	}
}

type chatRequestMessage struct {
	Role       string
	Content    string
	ToolCallID string `json:"tool_call_id"`
	Reasoning  string `json:"reasoning_content"`
}

func (m *chatRequestMessage) UnmarshalJSON(data []byte) error {
	var wire struct {
		Role       string
		Content    json.RawMessage
		ToolCallID string `json:"tool_call_id"`
		Reasoning  string `json:"reasoning_content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	m.Role, m.ToolCallID, m.Reasoning = wire.Role, wire.ToolCallID, wire.Reasoning
	if len(wire.Content) == 0 {
		return nil
	}
	if err := json.Unmarshal(wire.Content, &m.Content); err == nil {
		return nil
	}
	var blocks []struct{ Text string }
	if err := json.Unmarshal(wire.Content, &blocks); err != nil {
		return err
	}
	for _, block := range blocks {
		m.Content += block.Text
	}
	return nil
}
