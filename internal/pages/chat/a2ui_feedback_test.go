package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
)

func feedbackStream(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	delta, _ := json.Marshal(map[string]string{"role": "assistant", "content": text})
	fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":%s}]}\n\ndata: [DONE]\n\n", delta)
}

func TestA2UIOutputFailuresReachModelAndCanBeCorrected(t *testing.T) {
	for _, tc := range []struct{ name, output, reason string }{
		{"duplicate surface", `<a2ui>surface("existing")
root = Text("Duplicate")</a2ui>`, `surface \"existing\" already exists`},
		{"JSON syntax", `<a2ui-json>broken</a2ui-json>`, "A2UI_OUTPUT_ERROR"},
		{"Express syntax", `<a2ui>surface("new")
root = Text(</a2ui>`, "A2UI_OUTPUT_ERROR"},
		{"unknown component", `<a2ui>surface("new")
root = UnrecognizedWidget()</a2ui>`, "unknown component"},
		{"unknown surface", `<a2ui-json>{"version":"1.0","updateDataModel":{"surfaceId":"missing","path":"/value","value":1}}</a2ui-json>`, "INTEGRITY_ERROR"},
		{"protocol validation", `<a2ui-json>{"version":"2.0","createSurface":{"surfaceId":"new"}}</a2ui-json>`, "A2UI_OUTPUT_ERROR"},
		{"unclosed block", `<a2ui>surface("new")`, "unclosed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct{ Messages []chatRequestMessage }
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				if calls.Add(1) == 1 {
					feedbackStream(w, tc.output)
					return
				}
				last := req.Messages[len(req.Messages)-1].Content
				if !strings.Contains(last, tc.reason) {
					t.Errorf("model did not receive %q: %s", tc.reason, last)
				}
				feedbackStream(w, `<a2ui>surface("existing", update=true)
root = Button(child=label, action=Event("continue"))
label = Text("Continue")</a2ui>`)
			}))
			defer server.Close()
			store, err := openLocalStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "feedback-test"})
			var initial []a2ui.AgentMessage
			if err := json.Unmarshal([]byte(`[{"version":"1.0","createSurface":{"surfaceId":"existing","catalogId":"https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json","components":[{"id":"root","component":"Text","text":"Original"}]}}]`), &initial); err != nil {
				t.Fatal(err)
			}
			prior := &turn{message: message{Role: "assistant"}, Interface: []interfaceUpdate{{Messages: initial}}}
			input := userTurn("Continue")
			reply := &turn{message: message{Role: "assistant"}}
			chat := &conversation{Turns: []*turn{prior, input, reply}}
			ctx := context.WithValue(t.Context(), interfaceContextKey{}, chat.interfaceRequest())
			question, err := backend.runSession(ctx, store, newID(), []*turn{prior, input}, nil, "", func(delta replyDelta) error {
				offset := len(reply.Content)
				reply.appendDelta(delta)
				chat.commitInterface(reply, offset, delta.interfaceUpdates)
				return nil
			})
			if err != nil || question != nil || calls.Load() != 2 {
				t.Fatalf("calls=%d question=%+v err=%v", calls.Load(), question, err)
			}
			var action a2ui.RendererMessage
			window := ui.NewTester(func(c *ui.Context) {
				a2uimygo.View(c, chat.interfaceView().state.Surface("existing"), a2uimygo.Options{OnAction: func(event a2ui.RendererMessage) { action = event }})
			}, 400, 400)
			if err := window.Click("Continue"); err != nil || action.Action == nil || action.Action.Name != "continue" {
				t.Fatalf("corrected control could not submit: %+v %v", action, err)
			}
		})
	}
}

func TestA2UIErrorCorrectionIsBoundedAndRetainsLastFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		feedbackStream(w, `<a2ui-json>broken</a2ui-json>`)
	}))
	defer server.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "feedback-test"})
	request := (&conversation{}).interfaceRequest()
	ctx := context.WithValue(t.Context(), interfaceContextKey{}, request)
	_, err = backend.runSession(ctx, store, newID(), []*turn{userTurn("Show an interface")}, nil, "", func(replyDelta) error { return nil })
	if err != nil || calls.Load() != 3 {
		t.Fatalf("correction did not stop after two retries: calls=%d err=%v", calls.Load(), err)
	}
	if len(request.feedback.take()) == 0 {
		t.Fatal("failure at the retry limit was discarded")
	}
}

func TestA2UIFeedbackQueueRetainsEveryErrorCategory(t *testing.T) {
	queue := new(interfaceFeedbackQueue)
	for _, message := range []string{"binding resolution failed", "media download failed", "action execution failed", "surface application failed"} {
		feedback := interfaceFeedback{err: errors.New(message)}
		queue.report("surface", feedback)
		queue.report("surface", feedback)
	}
	queue.report("surface", interfaceFeedback{err: &a2ui.ViewDiagnostic{Code: "CYCLIC_REFERENCE", SurfaceID: "surface", Component: "root"}})
	queue.report("surface", interfaceFeedback{event: a2ui.RendererMessage{Version: a2ui.Version, Error: &a2ui.RendererError{Code: "EXPRESSION_ERROR", FunctionCallID: "call-1", Message: "RPC failed"}}})
	inputs, err := queue.modelInputs()
	if err != nil || len(inputs) != 6 {
		t.Fatalf("errors dropped or duplicated: count=%d err=%v", len(inputs), err)
	}
	if len(queue.take()) != 0 {
		t.Fatal("already delivered errors remained queued")
	}
}

func TestA2UIRenderFailureIsQueuedOnceAndSentOnNextRequest(t *testing.T) {
	var messages []a2ui.AgentMessage
	source := `[{"version":"1.0","createSurface":{"surfaceId":"cycle","catalogId":"https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json","components":[{"id":"root","component":"Column","children":["card"]},{"id":"card","component":"Card","child":"root"}]}}]`
	if err := json.Unmarshal([]byte(source), &messages); err != nil {
		t.Fatal(err)
	}
	entry := &turn{message: message{Role: "assistant"}, State: "done", Interface: []interfaceUpdate{{Messages: messages}}}
	chat := &conversation{Turns: []*turn{entry}}
	// Exercise the renderer's fallback callback independently of strict state
	// validation, which normally rejects this cycle before mounting the view.
	view := projectInterface(nil)
	view.state.AllowViewFallbacks(true)
	view.apply(entry, 0, entry.Interface[0])
	chat.projectedInterface = view
	app := &chatApp{}
	window := ui.NewTester(func(c *ui.Context) {
		view := chat.interfaceView()
		app.renderInterfaceAnswer(c, chat, entry, view, view.activeRows(entry))
	}, 400, 400)
	window.Frame()
	window.Frame()
	queue := chat.interfaceFeedbackQueue()
	queue.mu.Lock()
	count := len(queue.pending)
	queue.mu.Unlock()
	if count != 1 {
		t.Fatalf("render failure was dropped or repeated on repaint: %d", count)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct{ Messages []chatRequestMessage }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		encoded, _ := json.Marshal(req.Messages)
		if !strings.Contains(string(encoded), "CYCLIC_REFERENCE") {
			t.Errorf("next request omitted renderer diagnostic: %s", encoded)
		}
		if req.Messages[len(req.Messages)-1].Content != "Continue" {
			t.Error("diagnostic replaced the actual user request")
		}
		feedbackStream(w, "Received.")
	}))
	defer server.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "feedback-test"})
	ctx := context.WithValue(t.Context(), interfaceContextKey{}, chat.interfaceRequest())
	_, err = backend.runSession(ctx, store, newID(), []*turn{entry, userTurn("Continue")}, nil, "", func(replyDelta) error { return nil })
	if err != nil || calls.Load() != 1 {
		t.Fatalf("calls=%d error=%v", calls.Load(), err)
	}
	if len(queue.take()) != 0 {
		t.Fatal("delivered renderer diagnostic was not cleared")
	}
}

func TestA2UIFeedbackDoesNotDropErrorsAfterEightBlocks(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Messages []chatRequestMessage }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if calls.Add(1) == 1 {
			var output strings.Builder
			for i := 0; i < 12; i++ {
				fmt.Fprintf(&output, "<a2ui>surface(\"bad%d\")\nroot = Unknown%d()</a2ui>\n", i, i)
			}
			feedbackStream(w, output.String())
			return
		}
		var diagnostics strings.Builder
		for _, msg := range req.Messages {
			if strings.HasPrefix(msg.Content, "A2UI output error (JSON):") {
				diagnostics.WriteString(msg.Content)
			}
		}
		for i := 0; i < 12; i++ {
			if !strings.Contains(diagnostics.String(), fmt.Sprintf("Unknown%d", i)) {
				t.Errorf("diagnostic %d was dropped", i)
			}
		}
		feedbackStream(w, "Received.")
	}))
	defer server.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "feedback-test"})
	ctx := context.WithValue(t.Context(), interfaceContextKey{}, (&conversation{}).interfaceRequest())
	_, err = backend.runSession(ctx, store, newID(), []*turn{userTurn("Show an interface")}, nil, "", func(replyDelta) error { return nil })
	if err != nil || calls.Load() != 2 {
		t.Fatalf("calls=%d error=%v", calls.Load(), err)
	}
}
