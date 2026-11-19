package storage

import (
	"database/sql"
	"errors"
)

type Writer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func PutData(db Writer, key string, value []byte) error {
	_, err := db.Exec(`
		INSERT INTO app_data (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func GetData(db *sql.DB, key string) ([]byte, error) {
	var value []byte
	err := db.QueryRow("SELECT value FROM app_data WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func PutConfig(db Writer, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func GetConfig(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}
