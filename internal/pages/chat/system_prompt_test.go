package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSystemPromptAppliesToExistingSession(t *testing.T) {
	const first, second = "Use {first}.", "Use {second}."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatRequestMessage }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) < 2 {
			t.Errorf("invalid model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		system := request.Messages[0]
		if system.Role != "system" || !strings.Contains(system.Content, "You are Microlight") || !strings.Contains(system.Content, "If the user denies a tool action") {
			t.Error("built-in instructions were missing")
		}
		last := request.Messages[len(request.Messages)-1].Content
		for step, prompt := range map[string]string{"first": first, "second": second} {
			count := strings.Count(system.Content, prompt)
			if count > 1 || (count == 1) != (last == step) {
				t.Errorf("%s instruction count = %d for %s", step, count, last)
			}
		}
		writeTestStream(w, "OK")
	}))
	defer server.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, reloaded := &chatApp{store: store}, &chatApp{store: store}
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test"})
	for _, step := range []struct{ message, prompt string }{{"first", first}, {"second", second}, {"cleared", ""}} {
		app.saveSystemPrompt(step.prompt)
		reloaded.loadSystemPrompt()
		if reloaded.systemPrompt != step.prompt {
			t.Fatalf("reloaded prompt = %q, want %q", reloaded.systemPrompt, step.prompt)
		}
		ctx := context.WithValue(t.Context(), systemPromptKey{}, reloaded.systemPrompt)
		if _, err := backend.runSession(ctx, store, "system-prompt-test", []*turn{userTurn(step.message)}, nil, "", func(replyDelta) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
}
