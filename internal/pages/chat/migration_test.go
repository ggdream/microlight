package chat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ggdream/microlight/internal/storage"
)

func TestRenameMigratesExistingDatabase(t *testing.T) {
	parent := t.TempDir()
	oldDir, newDir := filepath.Join(parent, "微光对话"), filepath.Join(parent, "微光")
	for _, dir := range []string{oldDir, newDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	old, err := openLocalStore(oldDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.saveSettings(modelSettings{BaseURL: defaultBaseURL, Model: defaultModel, APIKey: "migration-key"}); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(oldDir, storage.LegacyAgentName+".sqlite3")
	if err := os.Rename(filepath.Join(oldDir, storage.DatabaseName), legacy); err != nil {
		t.Fatal(err)
	}
	if err := storage.MigrateAppDatabase(newDir); err != nil {
		t.Fatal(err)
	}
	store, err := openLocalStore(newDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config, err := store.loadSettings()
	if err != nil || config == nil || config.APIKey != "migration-key" {
		t.Fatalf("previous connection settings were not copied: %v", err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal(err)
	}
}
