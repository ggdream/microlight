package chat

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/storage"

	"github.com/egoist/mygo/i18n"
)

func TestInterfaceLocaleMatchesSystemPreferences(t *testing.T) {
	supported := locales.Catalog.Languages()
	if len(supported) != 2 || supported[0] != "en" || supported[1] != "zh-Hans" {
		t.Fatalf("unexpected catalog languages: %v", supported)
	}
	for _, test := range []struct {
		preferred []string
		title     string
	}{
		{[]string{"zh_CN.UTF-8", "en-US"}, "设置"},
		{[]string{"en-GB", "zh-CN"}, "Settings"},
		{[]string{"fr-FR", "zh-Hans-CN"}, "设置"},
		{[]string{"zh-TW"}, "Settings"},
		{[]string{"fr-FR"}, "Settings"},
		{[]string{"C"}, "Settings"},
		{nil, "Settings"},
	} {
		tag := i18n.Match(test.preferred, supported)
		if title := locales.In(tag).Settings(); title != test.title {
			t.Errorf("system languages %v resolved to %q (%q), want %q", test.preferred, tag, title, test.title)
		}
	}
}

func TestConfigMigrationKeepsModelSettingsAndStoresLanguage(t *testing.T) {
	dir := t.TempDir()
	legacyWorkspace := filepath.Join(dir, "legacy-workspace")
	otherWorkspace := filepath.Join(dir, "other-workspace")
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			store.Close()
		}
	})
	if _, err := store.Exec(`DROP TABLE config;
		DROP TABLE adk_session_owners;
		DROP INDEX adk_checkpoints_by_session;
		ALTER TABLE adk_checkpoints DROP COLUMN session_id;
		ALTER TABLE turns DROP COLUMN activity;
        ALTER TABLE turns DROP COLUMN a2ui;
		ALTER TABLE conversations DROP COLUMN access_mode;
		ALTER TABLE conversations DROP COLUMN workspace;
		ALTER TABLE conversations DROP COLUMN execution_target;
		ALTER TABLE conversations DROP COLUMN pinned;
		ALTER TABLE conversations DROP COLUMN archived;
		ALTER TABLE conversations DROP COLUMN branch_id;
		ALTER TABLE conversations DROP COLUMN session_model;
		DROP TABLE conversation_branches;
		PRAGMA user_version = 6;`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec("INSERT INTO conversations (id, title, position) VALUES ('chat-1', 'Existing', 1)"); err != nil {
		t.Fatal(err)
	}
	legacySettings, err := json.Marshal(map[string]string{
		"base_url": "https://example.com/v1", "model": "test",
		"access_mode": "smart", "workspace": legacyWorkspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.PutData(store.DB, "settings", legacySettings); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutData(store.DB, "selected", []byte("chat-1")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	config, err := store.loadSettings()
	if err != nil || config == nil || config.Model != "test" {
		t.Fatalf("existing model settings were not migrated: %v", err)
	}
	if previous, err := store.getData("settings"); err != nil || previous != nil {
		t.Fatalf("old settings entry was not removed: %v", err)
	}
	if selected, err := store.getData("selected"); err != nil || string(selected) != "chat-1" {
		t.Fatalf("selected conversation was not preserved: %v", err)
	}
	h, err := store.loadHistory()
	if err != nil || len(h.Chats) != 1 || h.Chats[0].accessMode != localtools.Smart || h.Chats[0].workspace != legacyWorkspace {
		t.Fatalf("existing workspace and permission were not migrated: %+v, %v", h, err)
	}
	app := &chatApp{store: store, history: h}
	other := app.newConversation("Other", localtools.Ask, otherWorkspace)
	app.save(other)
	app.chooseAccess(other, localtools.Full, time.Now())
	if app.fullAccess == nil || other.accessMode != localtools.Ask {
		t.Fatal("selecting full access must request confirmation without changing the mode")
	}
	readyAt := app.fullAccess.readyAt
	app.confirmFullAccess(readyAt.Add(-time.Millisecond))
	if app.fullAccess == nil || other.accessMode != localtools.Ask {
		t.Fatal("confirmation must wait for the countdown")
	}
	app.selectChat("chat-1")
	app.confirmFullAccess(readyAt)
	if app.fullAccess != nil || other.accessMode != localtools.Ask || h.Chats[0].accessMode != localtools.Smart {
		t.Fatal("switching conversations must cancel confirmation")
	}
	app.selectChat(other.ID)
	app.chooseAccess(other, localtools.Full, time.Now())
	app.confirmFullAccess(app.fullAccess.readyAt)
	if err := storage.PutConfig(store.DB, "language", string(settings.English)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved, err := store.getConfig("language"); err != nil || saved != string(settings.English) {
		t.Fatalf("language was not restored from SQLite: %q, %v", saved, err)
	}
	h, err = store.loadHistory()
	if err != nil || len(h.Chats) != 2 || h.Chats[0].accessMode != localtools.Full ||
		h.Chats[0].workspace != otherWorkspace || h.Chats[1].accessMode != localtools.Smart ||
		h.Chats[1].workspace != legacyWorkspace {
		t.Fatalf("per-conversation policy did not survive restart: %+v, %v", h, err)
	}

}
