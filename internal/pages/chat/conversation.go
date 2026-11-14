package chat

import (
	"strings"
	"uuid"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
)

func (app *chatApp) loadHistory() {
	if app.store == nil {
		return
	}
	saved, err := app.store.loadHistory()
	if err != nil {
		app.historyErr = locales.HistoryReadFailed()
		return
	}
	app.history = saved
	for _, chat := range app.history.Chats {
		app.refreshDetachedCommands(chat)
		// Older versions stored only the first 22 characters of generated titles.
		if len(chat.Turns) > 0 && chat.Turns[0].Role == "user" {
			title := strings.Join(strings.Fields(chat.Turns[0].Content), " ")
			chars := []rune(title)
			if len(chars) > 22 && chat.Title == string(chars[:22])+"…" {
				chat.Title = title
			}
		}
		for index, entry := range chat.Turns {
			if entry.Role == "assistant" && chat.target.Kind != "" {
				records, err := app.store.loadRemoteCommands(chat.ID, chat.branchID, index)
				if err != nil {
					app.historyErr = locales.HistoryReadFailed()
				} else {
					entry.applyRemoteCommands(records)
				}
			}
			if index == len(chat.Turns)-1 {
				entry.resumeCommand = app.store.recoverableReplyCommand(chat, entry)
			}
			if entry.State == "pending" || (entry.State == "waiting" && !entry.canResume()) {
				entry.State = "stopped"
				if entry.resumeCommand == nil {
					chat.sessionID = ""
				}
			}
		}
	}
}

func (app *chatApp) save(changed *conversation) {
	if app.store == nil {
		return
	}
	if err := app.store.saveHistory(app.history, changed); err != nil {
		app.historyErr = locales.HistorySaveFailed()
		return
	}
	if changed != nil {
		changed.sentDraft = nil
	}
	app.historyErr = ""
}

func (app *chatApp) selected() *conversation {
	return app.visibleChat(app.history.Selected)
}

func (app *chatApp) visibleChat(id string) *conversation {
	for _, chat := range app.history.Chats {
		if chat.ID == id && !chat.archived {
			return chat
		}
	}
	return nil
}

func (app *chatApp) selectChat(id string) {
	if id != "" && app.visibleChat(id) == nil {
		return
	}
	if app.deferSettingsLeave(func() { app.selectChat(id) }) {
		return
	}
	if id != app.history.Selected && (!app.persistDraft() || !app.restoreDraft(id)) {
		return
	}
	app.closeSettings()
	app.fullAccess = nil
	if id != app.history.Selected {
		app.toolPanel = toolPreviewPanel{}
		app.resetComposerSkills()
		app.replySkills = nil
	}
	app.history.Selected = id
	app.closeWorkspaceForm()
	app.clearWorkspaceSelection()
	app.draftAccess, app.draftWorkspace, app.accessError = localtools.Ask, "", ""
	app.edit = nil
	app.queueEdit = nil
	app.save(nil)
}

func (app *chatApp) newConversation(title string, mode localtools.Mode, workspace string) *conversation {
	chat := &conversation{ID: newID(), Title: title, accessMode: mode.Normalized(), workspace: workspace}
	app.history.Chats = append([]*conversation{chat}, app.history.Chats...)
	app.history.Selected = chat.ID
	return chat
}

func newID() string {
	return uuid.NewV7().String()
}
