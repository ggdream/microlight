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
const previousDataDirName = "微光"
const AppDataDirName = "MicroLight"

const DatabaseName = "data/db.sqlite3"

// PrepareAppDataDir moves the previous data directory before the application opens it.
func PrepareAppDataDir(parent string) (string, error) {
	dir := filepath.Join(parent, AppDataDirName)
	info, err := os.Stat(dir)
	switch {
	case err == nil && info.IsDir():
		return dir, nil
	case err == nil:
		return "", fmt.Errorf("application data path is not a directory: %s", dir)
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}

	previous := filepath.Join(parent, previousDataDirName)
	info, err = os.Lstat(previous)
	if errors.Is(err, os.ErrNotExist) {
		return dir, nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("previous application data path is not a directory: %s", previous)
	}
	if err := os.Rename(previous, dir); err != nil {
		return "", fmt.Errorf("move previous application data: %w", err)
	}
	return dir, nil
}

func MigrateAppDatabase(dir string) error {
	target := filepath.Join(dir, DatabaseName)
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, source := range []string{
		filepath.Join(dir, "microlight.sqlite3"),
		filepath.Join(filepath.Dir(dir), previousDataDirName, DatabaseName),
		filepath.Join(filepath.Dir(dir), previousDataDirName, "microlight.sqlite3"),
		filepath.Join(filepath.Dir(dir), legacyAppName, LegacyAgentName+".sqlite3"),
	} {
		if source == target {
			continue
		}
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		return copyDatabase(source, target)
	}
	return nil
}

func copyDatabase(source, target string) error {
	db, err := sql.Open("sqlite", source)
	if err != nil {
		return err
	}
	defer db.Close()
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
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
