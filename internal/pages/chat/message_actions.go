package chat

import (
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/pages/chat/composer"
)

type messageEdit struct {
	entry               *turn
	input               composer.Input
	inComposer          bool
	previousDraft       string
	previousAttachments []*attachment.File
}

func (app *chatApp) beginMessageEdit(chat *conversation, entry *turn, compact bool) {
	if app.edit != nil && app.edit.inComposer {
		app.cancelComposerEdit()
	}
	if compact && !app.persistDraft() {
		return
	}
	edit := &messageEdit{entry: entry, inComposer: compact}
	edit.input.SetText(entry.Content)
	if compact {
		edit.previousDraft = app.draft.DraftMarkdown()
		edit.previousAttachments = slices.Clone(app.draftAttachments)
		app.draft.SetText(entry.Content)
		app.draft.Document.SetSelection(app.draft.Document.Text().Len(), app.draft.Document.Text().Len())
		app.draftAttachments = slices.Clone(entry.Attachments)
		app.focusComposer = true
	}
	app.edit = edit
}

func (app *chatApp) cancelComposerEdit() {
	if app.edit == nil || !app.edit.inComposer {
		return
	}
	app.draft.SetText(app.edit.previousDraft)
	app.draftAttachments = app.edit.previousAttachments
	app.edit = nil
	app.focusComposer = true
}

type messageBranch struct {
	id        string
	sessionID string
	turns     []*turn
}

func (app *chatApp) endReply(chat *conversation, reply *turn, state string) bool {
	if reply == nil {
		return false
	}
	switch {
	case chat.isRunning() && chat.run.reply == reply:
		chat.run.cancel()
		chat.run = nil
		reply.FinishedAt = time.Now()
	case state == "stopped" && chat.waitingReply() == reply:
		now := time.Now()
		reply.excludeWaitingTime(now)
		reply.FinishedAt = now
	default:
		return false
	}
	reply.finishInterrupted(state)
	chat.sessionID = ""
	return true
}

func (reply *turn) finishInterrupted(state string) {
	reply.closeBrowser()
	if len(reply.Clarifications) > 0 {
		last := &reply.Clarifications[len(reply.Clarifications)-1]
		last.CheckpointID, last.InterruptID = "", ""
		reply.clarificationSurface = nil
	}
	reply.activeTools = nil
	reply.compressing = false
	reply.keepProcessPreview()
	reply.State = state
}

func (reply *turn) closeBrowser() {
	if reply.browser != nil {
		reply.browser.Close()
		reply.browser = nil
	}
}

func (app *chatApp) stopReply(chat *conversation, reply *turn) {
	if app.endReply(chat, reply, "stopped") {
		app.save(chat)
	}
}

func (app *chatApp) canEditMessage(chat *conversation, index int) bool {
	return app.backend.configErr == nil && (!chat.isRunning() || index+2 == len(chat.Turns))
}

func (app *chatApp) resendMessage(chat *conversation, index int, edit *messageEdit, text string) bool {
	if app.deferBrowserAction(func() { app.resendMessage(chat, index, edit, text) }) {
		return true
	}
	if app.selected() != chat || app.edit != edit || index < 0 || index >= len(chat.Turns) ||
		chat.Turns[index] != edit.entry || !app.canEditMessage(chat, index) {
		return false
	}
	text = strings.TrimSpace(text)
	attachments := edit.entry.Attachments
	if edit.inComposer {
		attachments = slices.Clone(app.draftAttachments)
	}
	if text == "" && len(attachments) == 0 {
		return false
	}
	now := time.Now()
	entry := &turn{
		message:   message{Role: "user", Content: text, Attachments: attachments},
		StartedAt: now,
	}
	prefix := append(slices.Clone(chat.Turns[:index]), entry)
	if !app.forkReply(chat, index, prefix, now) {
		return false
	}
	app.edit = nil
	return true
}

func (app *chatApp) forkReply(chat *conversation, forkIndex int, prefix []*turn, now time.Time) bool {
	if app.deferBrowserAction(func() { app.forkReply(chat, forkIndex, prefix, now) }) {
		return true
	}
	original := messageBranch{id: chat.branchID, sessionID: chat.sessionID, turns: chat.Turns}
	if original.id == "" {
		original.id = chat.ID
	}
	if chat.isRunning() {
		// The previous run is canceled after the branch is safely saved.
		stopped := *chat.run.reply
		stopped.browser = nil
		stopped.Activity = slices.Clone(stopped.Activity)
		stopped.keepProcessPreview()
		stopped.State, stopped.FinishedAt = "stopped", now
		original.turns = slices.Clone(original.turns)
		original.turns[len(original.turns)-1] = &stopped
	}
	reply := &turn{message: message{Role: "assistant"}, State: "pending", StartedAt: now}
	edited := *chat
	edited.branchID, edited.sessionID = newID(), ""
	edited.Turns = append(prefix[:len(prefix):len(prefix)], reply)
	if app.store != nil && !app.historyChange(app.store.saveForkedHistory(app.history.Selected, &edited, original, forkIndex)) {
		return false
	}
	if chat.isRunning() {
		app.endReply(chat, chat.run.reply, "stopped")
	} else if waiting := chat.waitingReply(); waiting != nil {
		waiting.closeBrowser()
	}
	chat.branchID, chat.sessionID, chat.Turns = edited.branchID, edited.sessionID, edited.Turns
	chat.projectedInterface = nil
	for _, tab := range slices.Clone(app.toolPanel.tabs) {
		if !slices.Contains(chat.Turns, tab.entry) {
			app.toolPanel.closeTab(tab)
		}
	}
	if !slices.Contains(chat.Turns, app.toolPanel.round) {
		app.toolPanel.round = nil
	}
	app.runReply(chat, reply, len(prefix), "", app.backend, app.currentModelChoice())
	return true
}
