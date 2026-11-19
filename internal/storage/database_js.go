//go:build js && wasm

package storage

import (
	"database/sql"
	"github.com/ggdream/microlight/pkg/sqliteweb"
)

// Browser state is origin-local IndexedDB, not an OS path or localStorage.
func Open(string) (*sql.DB, error)                 { return sqliteweb.Open("microlight") }
func ReadLegacy(string, string, any) (bool, error) { return false, nil }
func PrepareAppDataDir(string) (string, error)     { return "", nil }
func MigrateAppDatabase(string) error              { return nil }
