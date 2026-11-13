package chat

import (
	"slices"
	"testing"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
)

func TestMessageQueueKeepsDraftsAndOrderAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store, backend: &Chat{}}
	app.loadBusySendMode()
	chat := app.newConversation("Queue", localtools.Ask, "")
	app.save(chat)
	app.active = chat
	app.draft.SetText("first")
	app.draftAttachments = []*attachment.File{{Name: "note.txt", Data: []byte("draft attachment")}}
	if !app.persistDraft() || !app.send(app.draft.Markdown(), false) || !app.send("second", false) || !app.send("remove me", false) {
		t.Fatal("could not queue messages during an active reply")
	}
	if len(chat.Turns) != 0 || app.active != chat || app.draft.Markdown() != "" || len(app.draftAttachments) != 0 {
		t.Fatal("queueing must clear the draft and leave the running reply and transcript intact")
	}
	edit := "edited second"
	if !app.updateQueuedMessage(chat, chat.queue[1].ID, &edit) || !app.updateQueuedMessage(chat, chat.queue[2].ID, nil) {
		t.Fatal("could not edit or remove a queued message")
	}
	app.chooseBusySendMode(busySendSteer)
	if app.shouldQueue(chat) {
		t.Fatal("direct steer mode still queues while running")
	}

	reopened, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reloaded := &chatApp{store: reopened}
	reloaded.loadBusySendMode()
	if reloaded.busySend != busySendSteer {
		t.Fatal("busy send preference did not persist")
	}
	history, err := reopened.loadHistory()
	if err != nil {
		t.Fatal(err)
	}
	saved := history.Chats[0]
	if len(saved.queue) != 2 || saved.queue[0].Text != "first" || saved.queue[1].Text != edit ||
		len(saved.queue[0].Attachments) != 1 || saved.queue[0].Attachments[0].Name != "note.txt" {
		t.Fatalf("unexpected restored queue: %+v", saved.queue)
	}
	if draft, err := reopened.getData(draftTextKey(saved.ID)); err != nil || len(draft) != 0 {
		t.Fatalf("enqueued message survived as a duplicate draft: %q (%v)", draft, err)
	}
	if draft, err := reopened.getData(draftImagesKey(saved.ID)); err != nil || len(draft) != 0 {
		t.Fatalf("enqueued attachment survived as a duplicate draft: %d bytes (%v)", len(draft), err)
	}
	first := saved.queue[0]
	saved.Turns = append(saved.Turns, &turn{message: message{Role: "user", Content: first.Text, Attachments: first.Attachments}},
		&turn{message: message{Role: "assistant"}, State: "pending"})
	if err := reopened.saveMessageQueue(saved.ID, saved.queue[1:], false, saved); err != nil {
		t.Fatal(err)
	}
	history, err = reopened.loadHistory()
	if err != nil {
		t.Fatal(err)
	}
	saved = history.Chats[0]
	if len(saved.Turns) != 2 || saved.Turns[0].Content != first.Text ||
		!slices.EqualFunc(saved.queue, chat.queue[1:], func(a, b queuedMessage) bool { return a.ID == b.ID }) {
		t.Fatal("starting the first item must move it to the transcript and leave the rest queued")
	}
}
