package chat

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func TestEditedBranchSurvivesReopenUnderOriginalChat(t *testing.T) {
	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	chat := &conversation{ID: newID(), Title: "Original title", sessionID: newID(), Turns: []*turn{
		userTurn("Retained prompt"),
		{message: message{Role: "assistant", Content: "Retained reply"}, State: "done"},
		userTurn("Old prompt"),
		{message: message{Role: "assistant", Content: "Old reply"}, State: "done"},
		{message: message{Role: "user", Attachments: []*attachment.File{{Name: "later.png", Data: []byte("image")}}}},
		{message: message{Role: "assistant", Content: "Later reply"}, State: "done"},
	}}
	h := history{Selected: chat.ID, Chats: []*conversation{chat}}
	if err := store.saveHistory(h, chat); err != nil {
		t.Fatal(err)
	}
	if err := store.saveDraft(chat.ID, "Unsent draft", true, nil, false); err != nil {
		t.Fatal(err)
	}
	original := messageBranch{id: chat.ID, sessionID: chat.sessionID, turns: chat.Turns}
	edited := *chat
	edited.branchID, edited.sessionID = newID(), newID()
	edited.Turns = append(slices.Clone(chat.Turns[:2]), userTurn("Edited prompt"),
		&turn{message: message{Role: "assistant", Content: "Edited reply"}, State: "done"})
	if err := store.saveForkedHistory(chat.ID, &edited, original, 2); err != nil {
		t.Fatal(err)
	}
	regenerated := edited
	regenerated.branchID, regenerated.sessionID = newID(), newID()
	regenerated.Turns = append(slices.Clone(edited.Turns[:3]),
		&turn{message: message{Role: "assistant", Content: "Regenerated reply"}, State: "done"})
	previous := messageBranch{id: edited.branchID, sessionID: edited.sessionID, turns: edited.Turns}
	if err := store.saveForkedHistory(chat.ID, &regenerated, previous, 3); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	h, err = reopened.loadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Chats) != 1 || h.Selected != chat.ID {
		t.Fatal("editing changed the sidebar conversations or selection")
	}
	current := h.Chats[0]
	if current.ID != chat.ID || current.Title != chat.Title || current.branchID != regenerated.branchID ||
		current.sessionID != regenerated.sessionID || len(current.Turns) != 4 ||
		current.Turns[2].Content != "Edited prompt" || current.Turns[3].Content != "Regenerated reply" {
		t.Fatalf("wrong active branch after reopening: %+v", current)
	}
	draft, err := reopened.getData(draftTextKey(chat.ID))
	if err != nil || string(draft) != "Unsent draft" {
		t.Fatalf("draft changed: %q (%v)", draft, err)
	}
	for _, edge := range []struct {
		child, parent string
		fork          int
	}{{edited.branchID, chat.ID, 2}, {regenerated.branchID, edited.branchID, 3}} {
		var parent string
		var fork int
		if err := reopened.QueryRow(`SELECT parent_id, fork_ordinal FROM conversation_branches
			WHERE id = ? AND conversation_id = ?`, edge.child, chat.ID).Scan(&parent, &fork); err != nil {
			t.Fatal(err)
		}
		if parent != edge.parent || fork != edge.fork {
			t.Fatalf("incorrect branch parent or fork: %s, %d", parent, fork)
		}
	}
	var session string
	var payload []byte
	if err := reopened.QueryRow(`SELECT session_id, turns FROM conversation_branches
		WHERE id = ? AND conversation_id = ?`, original.id, chat.ID).Scan(&session, &payload); err != nil {
		t.Fatal(err)
	}
	var snapshot []struct {
		Content     string
		Attachments []*attachment.File `json:"images"`
	}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if session != original.sessionID ||
		len(snapshot) != 6 || snapshot[2].Content != "Old prompt" ||
		len(snapshot[4].Attachments) != 1 || snapshot[4].Attachments[0].Name != "later.png" {
		t.Fatalf("original branch was not retained: %s", payload)
	}
}

func TestUserMessageCopyAndInlineEdit(t *testing.T) {
	entry := &turn{
		message:   message{Role: "user", Content: "Original text"},
		StartedAt: time.Date(2026, 11, 12, 10, 11, 42, 0, time.Local),
	}
	chat := &conversation{Turns: []*turn{entry, {}}}
	app := &chatApp{backend: &Chat{}}
	surface := ui.NewTester(func(c *ui.Context) { app.userMessage(c, chat, 0, entry) }, 620, 240)
	bubble, ok := surface.Find("Original text")
	if !ok {
		t.Fatal("user message missing")
	}
	if _, shown := surface.Find("10:11"); shown {
		t.Fatal("send time is visible before hovering")
	}
	surface.Move(bubble.X+bubble.W/2, bubble.Y+bubble.H/2)
	timestamp, shown := surface.Find("10:11")
	copyButton, found := surface.Find(locales.CopyMessage())
	if !shown || !found || timestamp.X >= copyButton.X || timestamp.Y <= bubble.Y {
		t.Fatal("send time should appear below the bubble and to the left of Copy")
	}
	if err := surface.Click(locales.CopyMessage()); err != nil {
		t.Fatal(err)
	}
	if surface.Clipboard() != entry.Content {
		t.Fatalf("unexpected clipboard: %q", surface.Clipboard())
	}
	if _, ok := surface.Find(locales.MessageCopied()); !ok {
		t.Fatal("copy acknowledgement missing")
	}
	if err := surface.Click(locales.EditMessage()); err != nil {
		t.Fatal(err)
	}
	if app.edit == nil || app.edit.input.Markdown() != "Original text" {
		t.Fatal("editor did not open with the original text")
	}
	if _, ok := surface.Find(locales.Resend()); !ok {
		t.Fatal("resend action missing")
	}
	if err := surface.Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	if app.edit != nil || entry.Content != "Original text" {
		t.Fatal("cancel changed the original message")
	}
}

func TestSteerKeepsOriginalPromptAndStopsOldRequest(t *testing.T) {
	user := &turn{message: message{Role: "user", Content: "Original prompt"}}
	reply := &turn{message: message{Role: "assistant"}, State: "pending"}
	followup := &turn{message: message{Role: "user", Content: "Change direction"}}
	chat := &conversation{Turns: []*turn{user, reply}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	app := &chatApp{active: chat, running: reply, cancel: cancel}
	if !app.endReply(chat, reply, "steered") || ctx.Err() == nil || reply.State != "steered" ||
		reply.FinishedAt.IsZero() || app.running != nil || app.active != nil {
		t.Fatal("steering did not end the old request")
	}
	chat.Turns = append(chat.Turns, followup)
	if got := replyInput(chat.Turns[:3]); !slices.Equal(got, []*turn{user, followup}) {
		t.Fatalf("unexpected history without body: %v", got)
	}
	reply.Content = "Partial reply"
	if got := replyInput(chat.Turns[:3]); !slices.Equal(got, []*turn{user, reply, followup}) {
		t.Fatalf("unexpected history with body: %v", got)
	}
}

func TestStopWaitingClarification(t *testing.T) {
	pausedAt := time.Now().Add(-time.Minute)
	reply := &turn{
		State: "waiting", StartedAt: pausedAt.Add(-2 * time.Second), FinishedAt: pausedAt,
		Clarifications: []clarification{{Question: "Where?", CheckpointID: "saved", InterruptID: "waiting"}},
	}
	chat := &conversation{Turns: []*turn{reply}}
	app := &chatApp{}
	app.stopReply(chat, reply)
	if chat.waitingReply() != nil || reply.State != "stopped" || !reply.FinishedAt.After(pausedAt) ||
		reply.FinishedAt.Sub(reply.StartedAt) != 2*time.Second ||
		reply.Clarifications[0].CheckpointID != "" || reply.Clarifications[0].InterruptID != "" {
		t.Fatal("stopping a clarification did not discard its checkpoint")
	}
}

func TestRegenerateConfirmationKeysStayInDialog(t *testing.T) {
	var draft string
	open, confirms, composerKeys := false, 0, 0
	window := ui.NewTester(func(c *ui.Context) {
		editor := ui.TextInputBase(c.Key("draft"), &draft).Label("Draft").Width(200).Height(30)
		if editor.Shortcut(0, ui.KeyEnter) || editor.Shortcut(0, ui.KeyEscape) {
			composerKeys++
		}
		ui.Button(c, "Regenerate").OnClick(func() { open = true })
		if open {
			var confirmed bool
			open, confirmed = regenerateConfirmation(c)
			if confirmed {
				confirms++
			}
		}
	}, 660, 440)
	show := func() {
		t.Helper()
		if err := window.Click("Draft"); err != nil {
			t.Fatal(err)
		}
		if err := window.Click("Regenerate"); err != nil {
			t.Fatal(err)
		}
	}
	show()
	window.Key(0, ui.KeyEscape)
	if open || confirms != 0 || composerKeys != 0 {
		t.Fatalf("Esc: open=%t confirms=%d composer keys=%d", open, confirms, composerKeys)
	}
	show()
	window.Key(0, ui.KeyEnter)
	if open || confirms != 1 || composerKeys != 0 {
		t.Fatalf("Enter: open=%t confirms=%d composer keys=%d", open, confirms, composerKeys)
	}
	show()
	window.Key(0, ui.KeyTab)
	if !window.Focused(locales.Cancel()) {
		t.Fatal("Tab did not focus Cancel")
	}
	window.Key(0, ui.KeyEnter)
	if open || confirms != 2 || composerKeys != 0 {
		t.Fatalf("Enter on Cancel: open=%t confirms=%d composer keys=%d", open, confirms, composerKeys)
	}
}
