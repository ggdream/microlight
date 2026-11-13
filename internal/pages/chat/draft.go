package chat

import (
	"encoding/json"
	"slices"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/internal/storage"
)

type savedDraft struct {
	version uint64
	text    string
	images  []*attachment.File
}

func draftTextKey(id string) string   { return "draft:text:" + id }
func draftImagesKey(id string) string { return "draft:images:" + id }

func (app *chatApp) restoreDraft(id string) bool {
	var text []byte
	var images []*attachment.File
	if app.store != nil {
		var err error
		text, err = app.store.getData(draftTextKey(id))
		if err != nil {
			app.draftErr = locales.DraftReadFailed()
			return false
		}
		data, err := app.store.getData(draftImagesKey(id))
		if err != nil || (len(data) > 0 && json.Unmarshal(data, &images) != nil) {
			app.draftErr = locales.DraftReadFailed()
			return false
		}
	}
	app.clearDraft()
	app.draft.Document.SetText(composer.RestoreSkillChips(string(text)))
	app.draftAttachments = images
	app.savedDraft = savedDraft{app.draft.Document.Version(), string(text), slices.Clone(images)}
	app.draftErr = ""
	return true
}

func (app *chatApp) persistDraft() bool {
	if app.store == nil {
		return true
	}
	version := app.draft.Document.Version()
	saved := &app.savedDraft
	text := saved.text
	if version != saved.version {
		text = app.draft.DraftMarkdown()
	}
	imagesChanged := !slices.Equal(saved.images, app.draftAttachments)
	if text == saved.text && !imagesChanged {
		saved.version = version
		return true
	}

	// Finish committing a previously sent draft before saving new text under the same key.
	if chat := app.selected(); chat != nil && chat.sentDraft != nil {
		app.save(chat)
		if chat.sentDraft != nil {
			return false
		}
	}
	if err := app.store.saveDraft(app.history.Selected, text, text != saved.text, app.draftAttachments, imagesChanged); err != nil {
		app.draftErr = locales.DraftSaveFailed()
		return false
	}
	*saved = savedDraft{version, text, slices.Clone(app.draftAttachments)}
	app.draftErr = ""
	return true
}

func (store *localStore) saveDraft(id, text string, textChanged bool, images []*attachment.File, imagesChanged bool) error {
	if !imagesChanged {
		return putDraftData(store.DB, draftTextKey(id), []byte(text))
	}
	var data []byte
	if len(images) > 0 {
		var err error
		data, err = json.Marshal(images)
		if err != nil {
			return err
		}
	}
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if textChanged {
		if err := putDraftData(tx, draftTextKey(id), []byte(text)); err != nil {
			return err
		}
	}
	if err := putDraftData(tx, draftImagesKey(id), data); err != nil {
		return err
	}
	return tx.Commit()
}

func putDraftData(db storage.Writer, key string, data []byte) error {
	if len(data) == 0 {
		_, err := db.Exec("DELETE FROM app_data WHERE key = ?", key)
		return err
	}
	return storage.PutData(db, key, data)
}
