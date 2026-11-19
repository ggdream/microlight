//go:build js && wasm

// Browser acceptance harness. It uses an isolated IndexedDB database, never the
// app's settings/history. See docs/web-storage.md for the runner.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"syscall/js"

	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/pkg/sqliteweb"
)

var db *sql.DB

func main() {
	js.Global().Set("storageCheck", js.FuncOf(func(_ js.Value, args []js.Value) any {
		op := args[0].String()
		executor := js.FuncOf(func(_ js.Value, callbacks []js.Value) any {
			resolve, reject := callbacks[0], callbacks[1]
			go func() {
				value, err := run(op)
				if err != nil {
					reject.Invoke(err.Error())
				} else {
					resolve.Invoke(value)
				}
			}()
			return nil
		})
		promise := js.Global().Get("Promise").New(executor)
		executor.Release()
		return promise
	}))
	select {}
}

func run(op string) (any, error) {
	ctx := context.Background()
	switch op {
	case "open":
		var err error
		db, err = sqliteweb.Open("sqliteweb_qa")
		if err != nil {
			return nil, err
		}
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS checks (id INTEGER PRIMARY KEY, text TEXT, data BLOB, counter INTEGER NOT NULL DEFAULT 0);
		CREATE TABLE IF NOT EXISTS types (i INTEGER, f REAL, t TEXT, n BLOB, e BLOB);`)
		return "opened", err
	case "seed":
		data := make([]byte, 12*1024*1024)
		for i := range data {
			data[i] = byte((i*37 + i/251) % 256)
		}
		insert, err := db.Exec("INSERT OR REPLACE INTO checks(id,text,data) VALUES(?,?,?)", int64(9007199254740993), "微光\x00IndexedDB", data)
		if err != nil {
			return nil, err
		}
		id, err := insert.LastInsertId()
		if err != nil || id != 9007199254740993 {
			return nil, fmt.Errorf("last insert id changed: %d, %v", id, err)
		}
		if _, err = db.Exec("DELETE FROM types"); err != nil {
			return nil, err
		}
		_, err = db.Exec("INSERT INTO types VALUES(?,?,?,?,?)", int64(9223372036854775807), float64(1), "", []byte(nil), []byte{})
		return "seeded", err
	case "verify":
		var id int64
		var text string
		var data []byte
		var count int
		if err := db.QueryRow("SELECT id,text,data,counter FROM checks WHERE id=?", int64(9007199254740993)).Scan(&id, &text, &data, &count); err != nil {
			return nil, err
		}
		if id != 9007199254740993 || text != "微光\x00IndexedDB" || len(data) != 12*1024*1024 {
			return nil, errors.New("data changed across persistence")
		}
		for i, value := range data {
			if value != byte((i*37+i/251)%256) {
				return nil, fmt.Errorf("blob changed at %d", i)
			}
		}
		var integer int64
		var real any
		var empty string
		var null, emptyBlob []byte
		if err := db.QueryRow("SELECT i,f,t,n,e FROM types").Scan(&integer, &real, &empty, &null, &emptyBlob); err != nil {
			return nil, err
		}
		if integer != 9223372036854775807 || real != float64(1) || empty != "" || null != nil || emptyBlob == nil || len(emptyBlob) != 0 {
			return nil, fmt.Errorf("SQLite value types changed: %d %T %q %#v %#v", integer, real, empty, null, emptyBlob)
		}
		var integrity string
		if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
			return nil, err
		}
		if integrity != "ok" {
			return nil, errors.New(integrity)
		}
		sum := sha256.Sum256(data)
		return map[string]any{"sha256": hex.EncodeToString(sum[:]), "counter": count, "integrity": integrity, "bytes": len(data)}, nil
	case "rollback":
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err = tx.Exec("UPDATE checks SET text='discarded',data=zeroblob(14000000)"); err != nil {
			return nil, err
		}
		return "rolled back", tx.Rollback()
	case "constraint":
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		_, err = tx.Exec("INSERT INTO checks(id,text) VALUES(?,?)", int64(9007199254740993), "duplicate")
		var sqlErr *sqliteweb.Error
		if !errors.As(err, &sqlErr) || sqlErr.Code&255 != 19 {
			return nil, fmt.Errorf("constraint code was lost: %v", err)
		}
		return "constraint preserved", tx.Rollback()
	case "duplicate":
		other, err := sqliteweb.Open("sqliteweb_qa")
		if err == nil {
			other.Close()
			return nil, errors.New("second pool opened in same worker")
		}
		return "duplicate rejected", nil
	case "pick":
		files, err := desktop.OpenBrowserFiles(false, ".txt", 1024)
		if err != nil || len(files) != 1 {
			return nil, fmt.Errorf("file picker failed: %v", err)
		}
		return map[string]any{"name": files[0].Name, "text": string(files[0].Data)}, nil
	case "download":
		return "downloaded", desktop.DownloadBrowserFile("storage-check.txt", []byte("original bytes\x00"))
	case "increment":
		for range 40 {
			if _, err := db.Exec("UPDATE checks SET counter=counter+1"); err != nil {
				return nil, err
			}
		}
		return "incremented", nil
	case "crash":
		if _, err := db.Exec("BEGIN; UPDATE checks SET text='uncommitted',data=zeroblob(16000000);"); err != nil {
			return nil, err
		}
		return "uncommitted", nil
	case "close":
		return "closed", db.Close()
	default:
		return nil, errors.New("unknown check")
	}
}
