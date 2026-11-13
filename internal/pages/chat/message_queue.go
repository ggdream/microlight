package chat

import (
	"encoding/json"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/internal/storage"
)

type busySendMode string

const (
	busySendQueue     busySendMode = "queue"
	busySendSteer     busySendMode = "steer"
	busySendConfigKey              = "busy_send"
)

type queuedMessage struct {
	ID          string             `json:"id"`
	Text        string             `json:"text"`
	Model       modelChoice        `json:"model_choice,omitzero"`
	Attachments []*attachment.File `json:"attachments,omitempty"`
}

type queuedMessageEdit struct {
	id    string
	input composer.Input
}

func (app *chatApp) loadBusySendMode() {
	app.busySend = busySendQueue
	if app.store == nil {
		return
	}
	saved, err := app.store.getConfig(busySendConfigKey)
	if err != nil {
		log.Printf("read busy send preference: %v", err)
		return
	}
	switch busySendMode(saved) {
	case busySendSteer:
		app.busySend = busySendSteer
	case busySendQueue:
	default:
		if err := storage.PutConfig(app.store.DB, busySendConfigKey, string(busySendQueue)); err != nil {
			log.Printf("save default busy send preference: %v", err)
		}
	}
}

func (app *chatApp) chooseBusySendMode(mode busySendMode) {
	if app.store == nil || storage.PutConfig(app.store.DB, busySendConfigKey, string(mode)) != nil {
		app.form.generalStatus.message, app.form.generalStatus.failed = locales.SettingsSaveFailed(), true
		return
	}
	app.busySend = mode
	app.form.generalStatus.message, app.form.generalStatus.failed = "", false
}

func (app *chatApp) shouldQueue(chat *conversation) bool {
	return chat != nil && (app.active == chat && app.busySend != busySendSteer ||
		app.active == nil && len(chat.queue) != 0)
}

func (app *chatApp) enqueueDraft(chat *conversation, text string) bool {
	entry := queuedMessage{ID: newID(), Text: text, Attachments: app.draftAttachments, Model: app.currentModelChoice()}
	queue := append(slices.Clone(chat.queue), entry)
	if !app.saveMessageQueue(chat.ID, queue, true, nil) {
		return false
	}
	chat.queue = queue
	app.clearDraft()
	return true
}

func (app *chatApp) updateQueuedMessage(chat *conversation, id string, text *string) bool {
	index := slices.IndexFunc(chat.queue, func(entry queuedMessage) bool { return entry.ID == id })
	if index < 0 {
		return false
	}
	queue := slices.Clone(chat.queue)
	if text == nil {
		queue = slices.Delete(queue, index, index+1)
	} else {
		value := strings.TrimSpace(*text)
		if value == "" && len(queue[index].Attachments) == 0 {
			return false
		}
		queue[index].Text = value
	}
	if !app.saveMessageQueue(chat.ID, queue, false, nil) {
		return false
	}
	chat.queue = queue
	if app.queueEdit != nil && app.queueEdit.id == id {
		app.queueEdit = nil
	}
	return true
}

func (app *chatApp) startNextQueued(chat *conversation) {
	if len(chat.queue) != 0 {
		app.startQueuedMessage(chat, chat.queue[0].ID, false)
	}
}

func (app *chatApp) startQueuedMessage(chat *conversation, id string, steer bool) {
	if chat.archived ||
		(app.queueEdit != nil && app.queueEdit.id == id) {
		return
	}
	if steer {
		if app.active != chat || app.running == nil {
			return
		}
	} else if app.active != nil || chat.waitingReply() != nil || len(chat.queue) == 0 || chat.queue[0].ID != id {
		return
	}
	index := slices.IndexFunc(chat.queue, func(entry queuedMessage) bool { return entry.ID == id })
	if index < 0 {
		return
	}
	entry := chat.queue[index]
	choice := entry.Model
	backend, err := app.backendFor(choice)
	if err != nil {
		app.providerError = err.Error()
		return
	}
	if choice.ProviderID == "" {
		choice = app.currentModelChoice()
	}
	queue := slices.Delete(slices.Clone(chat.queue), index, index+1)
	now := time.Now()
	user := &turn{message: message{Role: "user", Content: entry.Text, Attachments: entry.Attachments}, StartedAt: now}
	reply := &turn{message: message{Role: "assistant"}, State: "pending", StartedAt: now}
	next := *chat
	next.Turns = append(slices.Clone(chat.Turns), user, reply)
	if steer {
		// Save the interrupted transcript before canceling the running request.
		previous := *app.running
		previous.Activity = slices.Clone(previous.Activity)
		previous.Clarifications = slices.Clone(previous.Clarifications)
		previous.FinishedAt = now
		previous.finishInterrupted("steered")
		next.Turns[len(chat.Turns)-1] = &previous
		next.sessionID = ""
	}
	if !app.saveMessageQueue(chat.ID, queue, false, &next) {
		return
	}
	if steer {
		app.endReply(chat, app.running, "steered")
		next.Turns[len(chat.Turns)-1] = chat.Turns[len(chat.Turns)-1]
	}
	chat.queue, chat.Turns = queue, next.Turns
	app.runReply(chat, reply, len(chat.Turns)-1, "", backend, choice)
}

func (app *chatApp) saveMessageQueue(id string, queue []queuedMessage, clearDraft bool, started *conversation) bool {
	return app.store == nil || app.historyChange(app.store.saveMessageQueue(id, queue, clearDraft, started))
}

func messageQueueKey(id string) string { return "queue:" + id }

func (store *localStore) saveMessageQueue(id string, queue []queuedMessage, clearDraft bool, started *conversation) error {
	var data []byte
	if len(queue) != 0 {
		var err error
		data, err = json.Marshal(queue)
		if err != nil {
			return err
		}
	}
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if started != nil {
		if err := putActiveHistory(tx, started); err != nil {
			return err
		}
	}
	if err := putDraftData(tx, messageQueueKey(id), data); err != nil {
		return err
	}
	if clearDraft {
		if _, err := tx.Exec("DELETE FROM app_data WHERE key IN (?, ?)", draftTextKey(id), draftImagesKey(id)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (store *localStore) loadMessageQueues(h history) error {
	chats := make(map[string]*conversation, len(h.Chats))
	for _, chat := range h.Chats {
		chats[chat.ID] = chat
	}
	rows, err := store.Query(`
		SELECT c.id, d.value FROM conversations c
		JOIN app_data d ON d.key = 'queue:' || c.id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return err
		}
		if err := json.Unmarshal(data, &chats[id].queue); err != nil {
			return err
		}
	}
	return rows.Err()
}
