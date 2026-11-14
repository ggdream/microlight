package chat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ggdream/microlight/internal/attachment"
)

func TestImagesStayInConversationAfterReopen(t *testing.T) {
	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Recreate the schema that an existing installation upgrades from.
	if _, err := store.Exec(`
		DROP TABLE turn_images;
		ALTER TABLE turns DROP COLUMN activity;
        ALTER TABLE turns DROP COLUMN a2ui;
		ALTER TABLE turns DROP COLUMN reasoning;
		ALTER TABLE turns DROP COLUMN started_at;
		ALTER TABLE turns DROP COLUMN finished_at;
		ALTER TABLE turns DROP COLUMN clarifications;
		ALTER TABLE conversations DROP COLUMN session_id;
		ALTER TABLE conversations DROP COLUMN session_model;
		ALTER TABLE conversations DROP COLUMN access_mode;
		ALTER TABLE conversations DROP COLUMN workspace;
		ALTER TABLE conversations DROP COLUMN execution_target;
		ALTER TABLE conversations DROP COLUMN pinned;
		ALTER TABLE conversations DROP COLUMN archived;
		ALTER TABLE conversations DROP COLUMN branch_id;
		DROP TABLE conversation_branches;
		DROP TABLE adk_session_events;
		DROP TABLE adk_session_owners;
		DROP TABLE adk_checkpoints;
		DROP TABLE config;
		PRAGMA user_version = 1;
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()

	var attachments []*attachment.File
	for i, pixel := range []color.RGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
		picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
		picture.SetRGBA(0, 0, pixel)
		var data bytes.Buffer
		if err := png.Encode(&data, picture); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, fmt.Sprintf("photo-%d.png", i))
		if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		attachment, err := attachment.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		attachments = append(attachments, attachment)
	}
	chat := &conversation{ID: "pictures", Title: "Pictures", Turns: []*turn{
		{message: message{Role: "user", Attachments: attachments}},
		{message: message{Role: "assistant", Content: "Red and blue."}, State: "done"},
	}}
	h := history{Selected: chat.ID, Chats: []*conversation{chat}}
	for range 2 {
		if err := upgraded.saveHistory(h, chat); err != nil {
			t.Fatal(err)
		}
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.loadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if restored.Selected != chat.ID || len(restored.Chats) != 1 {
		t.Fatal("conversation was not restored")
	}
	turns := restored.Chats[0].Turns
	if len(turns) != 2 || len(turns[0].Attachments) != 2 {
		t.Fatal("turns or attachments were not restored")
	}
	for i, attachment := range turns[0].Attachments {
		if attachment.Name != attachments[i].Name || !bytes.Equal(attachment.Data, attachments[i].Data) {
			t.Fatalf("attachment %d changed after reopening", i)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 3 {
			t.Errorf("invalid chat request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var parts []struct {
			Type     string
			Text     string
			ImageURL struct{ URL string } `json:"image_url"`
		}
		if err := json.Unmarshal(request.Messages[0].Content, &parts); err != nil || len(parts) != 3 {
			t.Errorf("expected text and two images: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if parts[0].Type != "text" || parts[0].Text == "" {
			t.Error("image-only message is missing its default prompt")
		}
		for i, attachment := range attachments {
			want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(attachment.Data)
			if parts[i+1].Type != "image_url" || parts[i+1].ImageURL.URL != want {
				t.Errorf("image %d changed in the request", i)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"The second image is blue.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	var reply string
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "vision-test"})
	err = backend.stream(t.Context(), []message{turns[0].message, turns[1].message, {Role: "user", Content: "What color is the second image?"}}, func(part replyDelta) error {
		reply += part.text
		return nil
	})
	if err != nil || reply != "The second image is blue." {
		t.Fatalf("unexpected follow-up response %q: %v", reply, err)
	}
}
