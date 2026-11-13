package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/proxy"

	"github.com/egoist/mygo/ui"
)

func TestSettingsPagesSaveIndependently(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	app.loadSettings()
	app.openSettings()
	app.form.model = "new-model"
	app.form.proxy.Mode = proxy.Direct

	window := ui.NewTester(app.settingsPage, 1000, 750)
	if !window.HasText(locales.ProxyNetwork()) || window.HasText(locales.ModelID()) {
		t.Fatal("General should contain network settings and exclude model settings")
	}
	if err := window.Click(locales.SaveSettings()); err != nil {
		t.Fatal(err)
	}
	if app.settings.Proxy.Mode != proxy.Direct || app.settings.Model != defaultModel {
		t.Fatal("saving General should apply the proxy without saving the model draft")
	}
	app.form.proxy = proxy.Config{Mode: proxy.Custom}
	if err := window.Click(locales.SettingsModel()); err != nil {
		t.Fatal(err)
	}
	if !window.HasText(locales.ModelID()) || !window.HasText(locales.TestConnection()) || window.HasText(locales.ProxyNetwork()) {
		t.Fatal("Model should contain model settings and exclude network settings")
	}
	if err := window.Click(locales.SaveSettings()); err != nil {
		t.Fatal(err)
	}
	if app.settings.Model != "new-model" || app.settings.Proxy.Mode != proxy.Direct {
		t.Fatal("saving Model should apply the model without saving the proxy draft")
	}
}

func TestSavedSettingsDriveNextRequest(t *testing.T) {
	for _, tc := range []struct{ name, apiKey string }{
		{"without_key", ""},
		{"with_key", "test-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiKey := tc.apiKey
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				wantAuth := ""
				if apiKey != "" {
					wantAuth = "Bearer " + apiKey
				}
				if r.Header.Get("Authorization") != wantAuth {
					t.Error("unexpected Authorization header")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				for field, want := range map[string]any{
					"model": "test-model", "temperature": float64(0), "top_p": float64(0),
					"max_tokens": float64(64), "stream": true,
				} {
					if body[field] != want {
						t.Errorf("%s = %v, want %v", field, body[field], want)
					}
				}
				if _, ok := body["thinking"]; ok {
					t.Error("unexpected DeepSeek-specific parameter")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()

			dir := t.TempDir()
			store, err := openLocalStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			app := &chatApp{store: store}
			app.loadSettings()
			app.openSettings()
			app.form.baseURL, app.form.model, app.form.apiKey = server.URL+"/v1/", "test-model", apiKey
			app.form.temperature, app.form.topP, app.form.maxTokens = "0", "0", "64"
			app.saveSettings()
			if app.form.failed {
				t.Fatalf("save settings: %s", app.form.message)
			}
			if info, err := os.Stat(filepath.Join(dir, "microlight.sqlite3")); err != nil {
				t.Fatal(err)
			} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
				t.Error("settings file allows access by other users")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopenedStore, err := openLocalStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopenedStore.Close()
			reopened := &chatApp{store: reopenedStore}
			reopened.loadSettings()
			reopened.openSettings()
			if reopened.form.apiKey != "" || reopened.connectionConfig().APIKey != apiKey {
				t.Fatal("saved key was exposed in the form or could not be reused")
			}
			reopened.form.baseURL = "http://example.test"
			if reopened.connectionConfig().APIKey != "" {
				t.Fatal("saved key followed a changed endpoint")
			}
			for _, backend := range []*Chat{app.backend, reopened.backend} {
				var reply strings.Builder
				err := backend.stream(t.Context(), []message{{Role: "user", Content: "ping"}}, func(part replyDelta) error {
					reply.WriteString(part.text)
					return nil
				})
				if err != nil || reply.String() != "OK" {
					t.Fatalf("reply = %q, err = %v", reply.String(), err)
				}
			}
		})
	}
}

func TestInvalidSettingsKeepActiveClient(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	app.loadSettings()
	backend := app.backend
	app.openSettings()
	app.form.temperature = "NaN"
	app.saveSettings()
	if !app.form.failed || app.backend != backend {
		t.Fatal("invalid temperature replaced the active client")
	}
	if config, err := store.loadSettings(); config != nil || err != nil {
		t.Fatal("invalid settings were written")
	}
}

func TestLegacyHistoryMigratesAndRestores(t *testing.T) {
	dir := t.TempDir()
	old := []byte(`{"selected":"older","chats":[
		{"id":"newer","title":"New","turns":[{"role":"user","content":"hello"},{"role":"assistant","content":"partial","state":"pending"}]},
		{"id":"older","title":"Old","turns":[]}]}`)
	if err := os.WriteFile(filepath.Join(dir, "conversations.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	app.loadHistory()
	if app.history.Selected != "older" || len(app.history.Chats) != 2 {
		t.Fatalf("unexpected restored history: %+v", app.history)
	}
	chat := app.history.Chats[0]
	if chat.ID != "newer" || len(chat.Turns) != 2 || chat.Turns[1].State != "stopped" {
		t.Fatal("history order or interrupted reply was not restored")
	}
	chat.Turns[1].Content = "finished"
	chat.Turns[1].State = "done"
	chat.Turns[1].Reasoning = "reasoning survived"
	chat.Turns[1].Activity = []timeline.Entry{{Tool: localtools.Read, Target: "sample.txt"}}
	chat.Turns[1].StartedAt = time.UnixMilli(1700000001000)
	chat.Turns[1].FinishedAt = time.UnixMilli(1700000002450)
	app.history.Selected = chat.ID
	app.save(chat)
	reopened, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	h, err := reopened.loadHistory()
	if err != nil || h.Selected != chat.ID || len(h.Chats) != 2 || h.Chats[0].Turns[1].Content != "finished" {
		t.Fatalf("updated history did not survive reopening: %v", err)
	}
	reply := h.Chats[0].Turns[1]
	if len(reply.Activity) != 1 || reply.Activity[0] != (timeline.Entry{Tool: localtools.Read, Target: "sample.txt"}) {
		t.Fatalf("tool activity did not survive reopening: %+v", reply.Activity)
	}
	if reply.Reasoning != "reasoning survived" || reply.FinishedAt.Sub(reply.StartedAt) != 1450*time.Millisecond ||
		!h.Chats[0].Turns[0].StartedAt.IsZero() {
		t.Fatal("stored reasoning, timing, or unknown legacy timestamp changed on reopen")
	}
}
