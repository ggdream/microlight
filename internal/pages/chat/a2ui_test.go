package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
)

func TestChatA2UIStreamActionAndRestore(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatRequestMessage }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Errorf("invalid request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		first, last := request.Messages[0].Content, request.Messages[len(request.Messages)-1].Content
		if !strings.Contains(first, "Prefer Express") {
			t.Error("A2UI instructions missing")
		}
		chunks := []string{"Before\n\n<a2", "ui>\nsurface(\"demo\", sendDataModel=true)\n" +
			"root = Column([field, submit])\n$/name = \"Original\"\n" +
			"field = TextField(label=\"Name\", value=$/name)\nlabel = Text(\"Confirm\")\n" +
			"submit = Button(child=label, action=Event(\"confirm\", {name: $/name}))\n</a2ui>",
			"\n\n<a2ui-json>broken</a2ui-json>\n\nAfter",
		}
		if calls.Add(1) > 1 {
			if len(request.Messages) < 4 || !strings.Contains(last, `"name":"Ada"`) ||
				!strings.Contains(last, `"sourceComponentId":"submit"`) || !strings.Contains(first, `"name":"Ada"`) {
				t.Errorf("runner lost event, user data or history: %s", last)
			}
			chunks = []string{`<a2ui-json>[{"version":"1.0","updateDataModel":{"surfaceId":"demo","path":"/name","value":"Done"}}]</a2ui-json>`}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range chunks {
			delta, _ := json.Marshal(map[string]string{"role": "assistant", "content": chunk})
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":%s}]}\n\n", delta)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "a2ui"})
	chat := &conversation{ID: newID(), sessionID: newID()}
	run := func(input *turn) *turn {
		t.Helper()
		chat.Turns = append(chat.Turns, input)
		reply := &turn{message: message{Role: "assistant"}}
		chat.Turns = append(chat.Turns, reply)
		ctx := context.WithValue(t.Context(), interfaceContextKey{}, chat.interfaceRequest())
		pending, err := backend.runSession(ctx, store, chat.sessionID, []*turn{input}, nil, "", func(delta replyDelta) error {
			offset := len(reply.Content)
			reply.appendDelta(delta)
			chat.commitInterface(reply, offset, delta.interfaceUpdates)
			return nil
		})
		if pending != nil || err != nil {
			t.Fatalf("run: %+v, %v", pending, err)
		}
		reply.State = "done"
		return reply
	}
	reply := run(userTurn("Show me a form"))
	if len(reply.Interface) != 2 || reply.Interface[0].Offset != len("Before\n\n") || !reply.Interface[1].Invalid ||
		strings.Contains(reply.Content, "a2ui") || !strings.Contains(reply.Content, "After") {
		t.Fatalf("incorrect text and surface ordering: %q %+v", reply.Content, reply.Interface)
	}
	surface := chat.interfaceView().state.Surface("demo")
	if surface == nil {
		t.Fatal("missing rendered surface")
	}
	var event a2ui.RendererMessage
	tester := ui.NewTester(func(c *ui.Context) {
		a2uimygo.View(c, surface, a2uimygo.Options{
			OnAction: func(action a2ui.RendererMessage) { event = action },
			OnDataChange: func(path string, value any) {
				chat.saveInterfaceData(reply, "demo", path, value)
			},
		})
	}, 640, 440)
	if err := tester.Click("Name"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type("Ada")
	if err := tester.Click("Confirm"); err != nil {
		t.Fatal(err)
	}
	if event.Action == nil || event.Action.Context["name"] != "Ada" {
		t.Fatalf("incorrect renderer event: %+v", event.Action)
	}
	h := history{Selected: chat.ID, Chats: []*conversation{chat}}
	if err := store.saveHistory(h, chat); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.loadHistory()
	if err != nil || len(restored.Chats) != 1 {
		t.Fatalf("restore: %+v, %v", restored, err)
	}
	chat = restored.Chats[0]
	reply = chat.Turns[1]
	action := &turn{message: message{Role: "user", Content: "Confirm"}, InterfaceAction: &event}
	run(action)
	surface = chat.interfaceView().state.Surface("demo")
	if got, _ := surface.Data().Get("/name"); got != "Done" {
		t.Fatalf("action did not update original surface: %v", got)
	}
	if got, _ := projectInterface(chat.Turns[:2]).state.Surface("demo").Data().Get("/name"); got != "Ada" {
		t.Fatalf("later update leaked into a branch prefix: %v", got)
	}
	if err := store.saveHistory(restored, chat); err != nil {
		t.Fatal(err)
	}
	restored, err = store.loadHistory()
	if err != nil || restored.Chats[0].Turns[2].InterfaceAction == nil {
		t.Fatalf("structured action not restored: %v", err)
	}

	app := &chatApp{backend: backend}
	tester = ui.NewTester(func(c *ui.Context) {
		app.renderInterfaceAnswer(c, chat, reply, chat.interfaceView(), chat.interfaceView().activeRows(reply))
	}, 640, 440)
	for _, label := range []string{"Before", "Confirm", locales.A2uiRenderFailed(), "After"} {
		if _, ok := tester.Find(label); !ok {
			t.Errorf("missing %q in reply", label)
		}
	}
	if event.Action.Timestamp.IsZero() {
		t.Error("invalid timestamp")
	}
}

func TestChatA2UIReportsUnavailableRendererFunction(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatRequestMessage }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Errorf("invalid request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body := `<a2ui-json>[{"version":"1.0","callRendererFunction":{"functionCallId":"open-1","callFunction":{"@call":"openUrl","catalogId":"https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json","args":{"url":"https://example.com"}}}}]</a2ui-json>`
		if count.Add(1) > 1 {
			last := request.Messages[len(request.Messages)-1].Content
			if !strings.Contains(last, `"functionCallId":"open-1"`) || !strings.Contains(last, `"code":"INVALID_FUNCTION_CALL"`) {
				t.Errorf("missing correlated response: %s", last)
			}
			body = "Click the link to open it."
		}
		chunk, _ := json.Marshal(map[string]string{"role": "assistant", "content": body})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":%s}]}\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "a2ui"})
	ctx := context.WithValue(t.Context(), interfaceContextKey{}, (&conversation{}).interfaceRequest())
	var reply turn
	question, err := backend.runSession(ctx, store, newID(), []*turn{userTurn("Open a link")}, nil, "", func(delta replyDelta) error {
		reply.appendDelta(delta)
		return nil
	})
	if err != nil || question != nil || count.Load() != 2 || reply.Content != "Click the link to open it." {
		t.Fatalf("unexpected renderer RPC flow: %v, %+v, %d requests, %q", err, question, count.Load(), reply.Content)
	}
}
