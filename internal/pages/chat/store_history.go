package chat

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/storage"
)

func putConversation(tx *sql.Tx, chat *conversation, position int) error {
	if chat == nil {
		return errors.New("missing conversation")
	}
	if _, err := tx.Exec(`
		INSERT INTO conversations (id, title, position, session_id, access_mode, workspace, branch_id)
		VALUES (?, ?, CASE WHEN ? > 0 THEN ? ELSE (SELECT COALESCE(MAX(position), 0) + 1 FROM conversations) END, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET title = excluded.title,
			session_id = excluded.session_id, access_mode = excluded.access_mode, branch_id = excluded.branch_id
	`, chat.ID, chat.Title, position, position, chat.sessionID, chat.accessMode.Normalized(), chat.workspace, chat.branchID); err != nil {
		return err
	}
	if chat.sessionID != "" {
		if _, err := tx.Exec(`
			INSERT INTO adk_session_owners (session_id, conversation_id) VALUES (?, ?)
			ON CONFLICT(session_id) DO NOTHING
		`, chat.sessionID, chat.ID); err != nil {
			return err
		}
	}
	for index, entry := range chat.Turns {
		if entry == nil {
			return errors.New("missing message")
		}
		var questions []byte
		if len(entry.Clarifications) != 0 {
			var err error
			questions, err = json.Marshal(entry.Clarifications)
			if err != nil {
				return err
			}
		}
		var activity []byte
		if len(entry.Activity) != 0 {
			var err error
			activity, err = json.Marshal(entry.Activity)
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`
			INSERT INTO turns (conversation_id, ordinal, role, content, state, error, reasoning, started_at, finished_at, clarifications, activity)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (conversation_id, ordinal) DO UPDATE SET
				role = excluded.role, content = excluded.content, state = excluded.state, error = excluded.error,
				reasoning = excluded.reasoning, started_at = excluded.started_at, finished_at = excluded.finished_at,
				clarifications = excluded.clarifications, activity = excluded.activity
		`, chat.ID, index, entry.Role, entry.Content, entry.State, entry.Error,
			entry.Reasoning, encodeTurnTime(entry.StartedAt), encodeTurnTime(entry.FinishedAt), questions, activity); err != nil {
			return err
		}
	}
	return nil
}

func encodeTurnTime(value time.Time) sql.NullInt64 {
	return sql.NullInt64{Int64: value.UnixMilli(), Valid: !value.IsZero()}
}

func decodeTurnTime(value sql.NullInt64) time.Time {
	if value.Valid {
		return time.UnixMilli(value.Int64)
	}
	return time.Time{}
}

func putAttachments(tx *sql.Tx, chat *conversation) error {
	// User messages and their attachments are immutable once sent.
	for turnIndex, entry := range chat.Turns {
		for index, attachment := range entry.Attachments {
			if _, err := tx.Exec(`
				INSERT INTO turn_images (conversation_id, turn_ordinal, ordinal, name, mime, data)
				VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (conversation_id, turn_ordinal, ordinal) DO NOTHING
			`, chat.ID, turnIndex, index, attachment.Name, attachment.MIME, attachment.Data); err != nil {
				return err
			}
		}
	}
	return nil
}

func (store *localStore) saveHistory(h history, changed *conversation) error {
	if changed == nil {
		return storage.PutData(store.DB, "selected", []byte(h.Selected))
	}
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, chat := range h.Chats {
		if chat == changed {
			if err := putActiveHistory(tx, chat); err != nil {
				return err
			}
			break
		}
	}
	if err := storage.PutData(tx, "selected", []byte(h.Selected)); err != nil {
		return err
	}
	return tx.Commit()
}

func putActiveHistory(tx *sql.Tx, chat *conversation) error {
	if err := putConversation(tx, chat, 0); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM turn_images WHERE conversation_id = ? AND turn_ordinal >= ?", chat.ID, len(chat.Turns)); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM turns WHERE conversation_id = ? AND ordinal >= ?", chat.ID, len(chat.Turns)); err != nil {
		return err
	}
	if err := putAttachments(tx, chat); err != nil {
		return err
	}
	if chat.sentDraft != nil {
		_, err := tx.Exec("DELETE FROM app_data WHERE key IN (?, ?)", draftTextKey(*chat.sentDraft), draftImagesKey(*chat.sentDraft))
		return err
	}
	return nil
}

func (store *localStore) saveForkedHistory(selected string, edited *conversation, original messageBranch, forkIndex int) error {
	type branchTurn struct {
		*turn
		Attachments []*attachment.File `json:"images,omitempty"`
	}
	turns := make([]branchTurn, len(original.turns))
	for i, entry := range original.turns {
		turns[i] = branchTurn{turn: entry, Attachments: entry.Attachments}
	}
	snapshot, err := json.Marshal(turns)
	if err != nil {
		return err
	}
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := putActiveHistory(tx, edited); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO conversation_branches (id, conversation_id, session_id, turns)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, turns = excluded.turns
	`, original.id, edited.ID, original.sessionID, snapshot); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO conversation_branches (id, conversation_id, parent_id, fork_ordinal)
		VALUES (?, ?, ?, ?)
	`, edited.branchID, edited.ID, original.id, forkIndex); err != nil {
		return err
	}
	if err := storage.PutData(tx, "selected", []byte(selected)); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *localStore) loadHistory() (history, error) {
	selected, err := store.getData("selected")
	if err != nil {
		return history{}, err
	}
	rows, err := store.Query(`
		SELECT c.id, c.title, c.session_id, c.branch_id, c.access_mode, c.workspace, c.pinned, c.archived, t.role, t.content, t.state, t.error, t.reasoning, t.started_at, t.finished_at, t.clarifications, t.activity
		FROM conversations c LEFT JOIN turns t ON t.conversation_id = c.id
		ORDER BY c.position DESC, c.id, t.ordinal
	`)
	if err != nil {
		return history{}, err
	}
	defer rows.Close()
	h := history{Selected: string(selected)}
	var chat *conversation
	for rows.Next() {
		var id, title string
		var sessionID, branchID, workspace string
		var mode localtools.Mode
		var pinned, archived bool
		var role, content, state, messageErr, reasoning sql.NullString
		var started, finished sql.NullInt64
		var questions, activity []byte
		if err := rows.Scan(&id, &title, &sessionID, &branchID, &mode, &workspace, &pinned, &archived, &role, &content, &state, &messageErr, &reasoning, &started, &finished, &questions, &activity); err != nil {
			return history{}, err
		}
		if chat == nil || chat.ID != id {
			chat = &conversation{ID: id, Title: title, sessionID: sessionID, branchID: branchID, accessMode: mode.Normalized(), workspace: workspace, pinned: pinned, archived: archived}
			h.Chats = append(h.Chats, chat)
		}
		if role.Valid {
			entry := &turn{
				message:    message{Role: role.String, Content: content.String},
				State:      state.String,
				Error:      messageErr.String,
				Reasoning:  reasoning.String,
				StartedAt:  decodeTurnTime(started),
				FinishedAt: decodeTurnTime(finished),
			}
			if len(questions) != 0 {
				if err := json.Unmarshal(questions, &entry.Clarifications); err != nil {
					return history{}, err
				}
			}
			if len(activity) != 0 {
				if err := json.Unmarshal(activity, &entry.Activity); err != nil {
					return history{}, err
				}
			}
			chat.Turns = append(chat.Turns, entry)
		}
	}
	if err := rows.Err(); err != nil {
		return history{}, err
	}
	rows.Close()
	if !slices.ContainsFunc(h.Chats, func(chat *conversation) bool { return chat.ID == h.Selected && !chat.archived }) {
		h.Selected = ""
	}
	if err := store.loadAttachments(h); err != nil {
		return history{}, err
	}
	return h, store.loadMessageQueues(h)
}

func (store *localStore) loadAttachments(h history) error {
	chats := make(map[string]*conversation, len(h.Chats))
	for _, chat := range h.Chats {
		chats[chat.ID] = chat
	}
	rows, err := store.Query(`
		SELECT conversation_id, turn_ordinal, name, mime, data
		FROM turn_images ORDER BY conversation_id, turn_ordinal, ordinal
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var index int
		attachment := &attachment.File{}
		if err := rows.Scan(&id, &index, &attachment.Name, &attachment.MIME, &attachment.Data); err != nil {
			return err
		}
		chat := chats[id]
		if chat == nil || index < 0 || index >= len(chat.Turns) {
			return errors.New("attachment without a matching turn")
		}
		chat.Turns[index].Attachments = append(chat.Turns[index].Attachments, attachment)
	}
	return rows.Err()
}

func (store *localStore) getData(key string) ([]byte, error) {
	return storage.GetData(store.DB, key)
}

func (store *localStore) getConfig(key string) (string, error) {
	return storage.GetConfig(store.DB, key)
}
