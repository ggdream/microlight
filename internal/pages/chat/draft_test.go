package chat

import (
	"bytes"
	"testing"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/markup"

	"github.com/egoist/mygo/ui"
)

func TestDraftRestoresAcrossReopenAndClearsWithSentMessage(t *testing.T) {
	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store, backend: &Chat{}, md: markup.New(ink)}
	app.restoreDraft("")
	// Drive the real editor without closing its window or switching conversations.
	window := ui.NewTester(app.view, 900, 600)
	const text = "草稿 **加粗** 和 $x^2$"
	window.Type(text)
	raw, err := store.getData(draftTextKey(""))
	if err != nil || string(raw) != text {
		t.Fatalf("typing did not save the Markdown draft: %q, %v", raw, err)
	}

	image := &attachment.File{Name: "draft.png", MIME: "image/png", Data: []byte{1, 2, 3}}
	app.draftAttachments = []*attachment.File{image}
	window.Frame()
	chat := &conversation{ID: "chat", Title: "Existing"}
	app.history.Chats = []*conversation{chat}
	app.save(chat)
	app.selectChat(chat.ID)
	app.draft.SetText("另一份草稿")
	window.Frame()
	app.selectChat("")
	if got := app.draft.Markdown(); got != text || len(app.draftAttachments) != 1 {
		t.Fatalf("switching conversations lost the draft: %q, %d images", got, len(app.draftAttachments))
	}

	reopened, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := &chatApp{store: reopened}
	restored.loadHistory()
	if !restored.restoreDraft(restored.history.Selected) || restored.draft.Markdown() != text ||
		len(restored.draftAttachments) != 1 || !bytes.Equal(restored.draftAttachments[0].Data, image.Data) {
		t.Fatal("reopening the database did not restore the draft and its attachment")
	}

	draftID := restored.history.Selected
	sent := restored.newConversation("Sent", localtools.Ask, "")
	sent.Turns = []*turn{{message: message{Role: "user", Content: text, Attachments: restored.draftAttachments}}}
	sent.sentDraft = &draftID
	restored.save(sent)
	if restored.historyErr != "" || sent.sentDraft != nil {
		t.Fatal("the send transaction failed")
	}
	restored.selectChat("")
	if restored.draft.Markdown() != "" || len(restored.draftAttachments) != 0 {
		t.Fatal("the sent draft returned after switching back to the composer")
	}
	restored.selectChat(chat.ID)
	if got := restored.draft.Markdown(); got != "另一份草稿" {
		t.Fatalf("sending a draft removed a different conversation's draft: %q", got)
	}
}
