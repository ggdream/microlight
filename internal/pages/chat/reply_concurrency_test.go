package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ggdream/microlight/internal/localtools"
)

func TestConversationsRunConcurrentlyAndStopIndependently(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	requests := make(chan string, 2)
	finishSecond := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatRequestMessage }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			http.Error(w, "invalid chat request", http.StatusBadRequest)
			return
		}
		prompt := request.Messages[len(request.Messages)-1].Content
		requests <- prompt
		select {
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			return
		case <-finishSecond:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		response, _ := json.Marshal(map[string]string{"role": "assistant", "content": "Reply to " + prompt})
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":%s}]}\n\ndata: [DONE]\n\n", response)
	}))
	defer server.Close()

	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{ctx: ctx, store: store, backend: newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test"})}
	updates := make(chan func(), 40)
	update := func(fn func()) { updates <- fn }
	start := func(prompt string) (*conversation, *turn) {
		chat := app.newConversation(prompt, localtools.Ask, "")
		reply := &turn{message: message{Role: "assistant"}, State: "pending"}
		chat.Turns = []*turn{userTurn(prompt), reply}
		app.startReply(chat, reply, 1, "", app.backend, modelChoice{}, update, func() {})
		return chat, reply
	}
	first, firstReply := start("first")
	second, secondReply := start("second")
	seen := map[string]bool{}
	for range 2 {
		select {
		case prompt := <-requests:
			seen[prompt] = true
		case <-ctx.Done():
			t.Fatal("both requests must reach the server before either is released")
		}
	}
	if !seen["first"] || !seen["second"] || first.sessionID == second.sessionID ||
		!first.isRunning() || !second.isRunning() {
		t.Fatal("concurrent replies did not retain separate sessions and active state")
	}
	app.stopReply(first, firstReply)
	if first.isRunning() || firstReply.State != "stopped" || !second.isRunning() {
		t.Fatal("stopping the first conversation affected the second")
	}
	close(finishSecond)
	for second.isRunning() {
		select {
		case fn := <-updates:
			fn()
		case <-ctx.Done():
			t.Fatal("second reply did not finish")
		}
	}
	if secondReply.State != "done" || secondReply.Content != "Reply to second" || firstReply.State != "stopped" {
		t.Fatalf("unexpected replies after stopping the first run: %s, %s %q", firstReply.State, secondReply.State, secondReply.Content)
	}
}
