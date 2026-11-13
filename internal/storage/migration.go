package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// These identifiers are only used to read installations predating the rename.
const LegacyAgentName = "\x77\x65\x69\x67\x75\x61\x6e\x67"
const legacyAppName = "微光对话"

const DatabaseName = "microlight.sqlite3"

func MigrateAppDatabase(dir string) error {
	target := filepath.Join(dir, DatabaseName)
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	source := filepath.Join(filepath.Dir(dir), legacyAppName, LegacyAgentName+".sqlite3")
	if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	db, err := sql.Open("sqlite", source)
	if err != nil {
		return err
	}
	defer db.Close()
	file, err := os.CreateTemp(dir, ".migration-*.sqlite3")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := db.Exec("VACUUM INTO ?", temp); err != nil {
		return fmt.Errorf("copy previous database: %w", err)
	}
	return os.Rename(temp, target)
}
