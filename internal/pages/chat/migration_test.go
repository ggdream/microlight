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

func TestDataDirectoryRenamePreservesStoreAndOtherFiles(t *testing.T) {
	parent := t.TempDir()
	previous := filepath.Join(parent, "微光")
	store, err := openLocalStore(previous)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.saveSettings(modelSettings{BaseURL: defaultBaseURL, Model: defaultModel, APIKey: "migration-key"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(previous, "window-state.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		dir, err := storage.PrepareAppDataDir(parent)
		if err != nil || dir != filepath.Join(parent, "MicroLight") {
			t.Fatalf("unexpected profile path: %q, %v", dir, err)
		}
		if err := storage.MigrateAppDatabase(dir); err != nil {
			t.Fatal(err)
		}
		migrated, err := openLocalStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		config, err := migrated.loadSettings()
		migrated.Close()
		if err != nil || config == nil || config.APIKey != "migration-key" {
			t.Fatalf("previous settings were not loaded: %v", err)
		}
		if data, err := os.ReadFile(filepath.Join(dir, "window-state.json")); err != nil || string(data) != "{}" {
			t.Fatalf("previous window state was not moved: %v", err)
		}
	}
}
