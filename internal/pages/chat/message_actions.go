package chat

import (
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/internal/pages/chat/composer"
)

type messageEdit struct {
	entry *turn
	input composer.Input
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
	case app.active == chat && app.running == reply:
		app.cancel()
		app.active, app.running, app.cancel, app.runningAccess = nil, nil, nil, nil
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
	if len(reply.Clarifications) > 0 {
		last := &reply.Clarifications[len(reply.Clarifications)-1]
		last.CheckpointID, last.InterruptID = "", ""
		reply.clarificationSurface = nil
	}
	reply.compressing = false
	reply.keepProcessPreview()
	reply.State = state
}

func (app *chatApp) stopReply(chat *conversation, reply *turn) {
	if app.endReply(chat, reply, "stopped") {
		app.save(chat)
	}
}

func (app *chatApp) canEditMessage(chat *conversation, index int) bool {
	return app.backend.configErr == nil && (app.active == nil ||
		(app.active == chat && index+2 == len(chat.Turns)))
}

func (app *chatApp) resendMessage(chat *conversation, index int, edit *messageEdit, text string) bool {
	if app.selected() != chat || app.edit != edit || index < 0 || index >= len(chat.Turns) ||
		chat.Turns[index] != edit.entry || !app.canEditMessage(chat, index) {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" && len(edit.entry.Attachments) == 0 {
		return false
	}
	now := time.Now()
	entry := &turn{
		message:   message{Role: "user", Content: text, Attachments: edit.entry.Attachments},
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
	original := messageBranch{id: chat.branchID, sessionID: chat.sessionID, turns: chat.Turns}
	if original.id == "" {
		original.id = chat.ID
	}
	if app.active == chat {
		// The previous run is canceled after the branch is safely saved.
		stopped := *app.running
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
	if app.active == chat {
		app.endReply(chat, app.running, "stopped")
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
