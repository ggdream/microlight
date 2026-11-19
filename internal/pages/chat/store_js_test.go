//go:build js && wasm

package chat

import (
	"bytes"
	"testing"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/session"
	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/pkg/sqliteweb"
)

// Run twice in the same browser context with a page reload between runs. The
// second run checks the app's real schema, settings, draft, history and BLOBs
// without reseeding them. The database name is separate from the application.
func TestBrowserStoreRoundTrip(t *testing.T) {
	db, err := sqliteweb.Open("microlight_app_qa")
	if err != nil {
		t.Fatal(err)
	}
	store := &localStore{Store: session.Store{DB: db}}
	defer store.Close()
	if err := store.migrate(""); err != nil {
		t.Fatal(err)
	}
	marker, err := store.getConfig("browser-check")
	if err != nil {
		t.Fatal(err)
	}
	const key = "isolated-storage-check-key"
	if marker == "" {
		file := &attachment.File{Name: "saved.bin", MIME: "application/octet-stream", Data: bytes.Repeat([]byte{0, 1, 2, 255}, 1024*1024)}
		chat := &conversation{ID: newID(), Title: "浏览器持久化验收", Turns: []*turn{
			{message: message{Role: "user", Content: "保留会话和附件", Attachments: []*attachment.File{file}}},
			{message: message{Role: "assistant", Content: "已保存"}, State: "done"},
		}}
		if err := store.saveHistory(history{Selected: chat.ID, Chats: []*conversation{chat}}, chat); err != nil {
			t.Fatal(err)
		}
		if err := store.saveDraft(chat.ID, "未发送的草稿", true, nil, false); err != nil {
			t.Fatal(err)
		}
		if err := storage.PutConfig(db, "browser-test-key", key); err != nil {
			t.Fatal(err)
		}
		if err := storage.PutConfig(db, "browser-check", "seeded"); err != nil {
			t.Fatal(err)
		}
		t.Log("seeded application schema, configuration, draft and attachment")
	} else {
		t.Log("checking application data after page reload")
	}
	h, err := store.loadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Chats) != 1 || h.Selected != h.Chats[0].ID || len(h.Chats[0].Turns) != 2 {
		t.Fatal("history changed")
	}
	files := h.Chats[0].Turns[0].Attachments
	if len(files) != 1 || !bytes.Equal(files[0].Data, bytes.Repeat([]byte{0, 1, 2, 255}, 1024*1024)) {
		t.Fatal("attachment changed")
	}
	draft, err := store.getData(draftTextKey(h.Selected))
	if err != nil || string(draft) != "未发送的草稿" {
		t.Fatalf("draft changed: %v", err)
	}
	saved, err := store.getConfig("browser-test-key")
	if err != nil || saved != key {
		t.Fatalf("configuration key changed: %v", err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 16 {
		t.Fatalf("schema version: %d, %v", version, err)
	}
}
