package chat

import (
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/assets/locales"
)

func (app *chatApp) clearDraft() {
	app.resetComposerSkills()
	app.draft.SetText("")
	app.draftAttachments = nil
	app.savedDraft = savedDraft{version: app.draft.Document.Version()}
	app.draftVersion++
	app.attachmentError = ""
}

func (app *chatApp) send(text string, steer bool) bool {
	text = strings.TrimSpace(text)
	if app.loadingAttachments || (text == "" && len(app.draftAttachments) == 0) || app.backend.configErr != nil {
		return false
	}
	chat := app.selected()
	if steer && (chat == nil || app.active != chat || !app.shouldQueue(chat)) {
		return false
	}
	if chat.waitingReply() != nil {
		return false
	}
	if chat != nil && chat.sentDraft != nil {
		app.save(chat)
		if chat.sentDraft != nil {
			return false
		}
	}
	if !steer && app.shouldQueue(chat) {
		return app.enqueueDraft(chat, text)
	}
	if app.active != nil {
		if app.active != chat || !app.endReply(chat, app.running, "steered") {
			return false
		}
	}
	draftID := app.history.Selected
	if chat == nil {
		title := strings.Join(strings.Fields(text), " ")
		if title == "" {
			title = locales.ImagesChat()
			if slices.ContainsFunc(app.draftAttachments, func(file *attachment.File) bool { return !file.IsImage() }) {
				title = locales.AttachmentsChat()
			}
		}
		chat = app.newConversation(title, app.draftAccess, app.draftWorkspace)
	}
	chat.Turns = append(chat.Turns, &turn{
		message:   message{Role: "user", Content: text, Attachments: app.draftAttachments},
		StartedAt: time.Now(),
	})
	chat.sentDraft = &draftID
	app.clearDraft()
	app.generate(chat, len(chat.Turns))
	return true
}
