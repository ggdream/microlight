package chat

import (
	"errors"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/storage"
)

func (app *chatApp) sidebarChats() []*conversation {
	chats := make([]*conversation, 0, len(app.history.Chats))
	for _, pinned := range []bool{true, false} {
		for _, chat := range app.history.Chats {
			if !chat.archived && chat.pinned == pinned {
				chats = append(chats, chat)
			}
		}
	}
	return chats
}

func (app *chatApp) historyChange(err error) bool {
	if err != nil {
		app.historyErr = locales.HistorySaveFailed()
		return false
	}
	app.historyErr = ""
	return true
}

func (app *chatApp) renameChat(chat *conversation, title string) bool {
	title = strings.TrimSpace(title)
	if title == "" || app.store == nil {
		return false
	}
	if !app.historyChange(updateChat(app.store, "UPDATE conversations SET title = ? WHERE id = ?", title, chat.ID)) {
		return false
	}
	chat.Title = title
	return true
}

func (app *chatApp) togglePin(chat *conversation) {
	if app.store != nil && app.historyChange(updateChat(app.store, "UPDATE conversations SET pinned = ? WHERE id = ?", !chat.pinned, chat.ID)) {
		chat.pinned = !chat.pinned
	}
}

func updateChat(db storage.Writer, query string, args ...any) error {
	result, err := db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		err = errors.New("conversation not found")
	}
	return err
}

func (app *chatApp) setChatArchived(chat *conversation, archived bool) {
	selected := archived && app.history.Selected == chat.ID
	if app.store == nil || chat.archived == archived || (selected && !app.persistDraft()) {
		return
	}
	if !app.historyChange(app.store.archiveChat(chat.ID, archived, selected)) {
		return
	}
	chat.archived = archived
	if selected {
		app.leaveRemovedChat()
	}
}

func (store *localStore) archiveChat(id string, archived, clearSelection bool) error {
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updateChat(tx, "UPDATE conversations SET archived = ? WHERE id = ?", archived, id); err != nil {
		return err
	}
	if clearSelection {
		if err := storage.PutData(tx, "selected", []byte{}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (app *chatApp) deleteChat(chat *conversation) bool {
	if app.store == nil || app.active == chat {
		return false
	}
	selected := app.history.Selected == chat.ID
	if !app.historyChange(app.store.deleteChat(chat, selected)) {
		return false
	}
	app.forgetWorkspaceCredential(chat.target.Credential)
	if chat.browser != nil {
		chat.browser.Close()
	}
	app.history.Chats = slices.DeleteFunc(app.history.Chats, func(entry *conversation) bool { return entry == chat })
	if selected {
		app.leaveRemovedChat()
	}
	return true
}

func (app *chatApp) leaveRemovedChat() {
	app.history.Selected = ""
	app.toolPanel = toolPreviewPanel{}
	app.fullAccess, app.retryReply, app.edit = nil, nil, nil
	app.queueEdit = nil
	app.closeWorkspaceForm()
	app.clearWorkspaceSelection()
	app.draftAccess, app.draftWorkspace, app.accessError = localtools.Ask, "", ""
	if !app.restoreDraft("") {
		app.clearDraft()
	}
}

func (store *localStore) deleteChat(chat *conversation, clearSelection bool) error {
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Older ADK runs encoded ownership in their ID prefixes.
	prefix := chat.ID + ":"
	if _, err := tx.Exec(`DELETE FROM adk_session_events
		WHERE session_id IN (SELECT session_id FROM adk_session_owners WHERE conversation_id = ?)
			OR substr(session_id, 1, length(?)) = ? OR session_id = ?`,
		chat.ID, prefix, prefix, chat.sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM adk_checkpoints
		WHERE session_id IN (SELECT session_id FROM adk_session_owners WHERE conversation_id = ?)
			OR substr(id, 1, length(?)) = ?
			OR (? <> '' AND substr(id, 1, length(?) + 1) = ? || ':')`,
		chat.ID, prefix, prefix, chat.sessionID, chat.sessionID, chat.sessionID); err != nil {
		return err
	}
	if reply := chat.waitingReply(); reply != nil {
		// Older versions used opaque checkpoint IDs.
		if _, err := tx.Exec("DELETE FROM adk_checkpoints WHERE id = ?", reply.Clarifications[len(reply.Clarifications)-1].CheckpointID); err != nil {
			return err
		}
	}
	for _, query := range []string{
		"DELETE FROM remote_commands WHERE conversation_id = ?",
		"DELETE FROM adk_session_owners WHERE conversation_id = ?",
		"DELETE FROM conversation_branches WHERE conversation_id = ?",
		"DELETE FROM turn_images WHERE conversation_id = ?",
		"DELETE FROM turns WHERE conversation_id = ?",
	} {
		if _, err := tx.Exec(query, chat.ID); err != nil {
			return err
		}
	}
	if err := updateChat(tx, "DELETE FROM conversations WHERE id = ?", chat.ID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM app_data WHERE key IN (?, ?, ?)", draftTextKey(chat.ID), draftImagesKey(chat.ID), messageQueueKey(chat.ID)); err != nil {
		return err
	}
	if clearSelection {
		if err := storage.PutData(tx, "selected", []byte{}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
