package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func TestProviderSelectionRoutesRequestsAndSurvivesRestart(t *testing.T) {
	requests := make(chan struct{ model, auth string }, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Model string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- struct{ model, auth string }{body.Model, r.Header.Get("Authorization")}
		writeTestStream(w, "OK")
	}))
	defer server.Close()

	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	legacy := modelSettings{BaseURL: server.URL, Model: "legacy-model", APIKey: "legacy-key"}
	if err := store.saveSettings(legacy); err != nil {
		t.Fatal(err)
	}
	app := &chatApp{store: store}
	app.loadSettings()
	legacyChoice := app.currentModelChoice()
	app.openSettings()
	if app.savedKeyForForm() != "legacy-key" || app.form.model != "legacy-model" {
		t.Fatal("legacy settings were not migrated into the first provider")
	}

	app.editProvider("")
	app.form.providerName, app.form.baseURL, app.form.model = "Second Provider", server.URL, "primary"
	app.form.apiKey, app.form.maxTokens = "second-key", "64"
	alternate := "alternate"
	app.form.extraModels = []*string{&alternate}
	app.saveSettings()
	if app.form.failed || len(app.providers.Providers) != 2 {
		t.Fatalf("second provider did not save: %s", app.form.message)
	}
	secondID := app.form.providerID
	app.closeSettings()

	window := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Gap(12).Children(func() {
			app.topbar(c.Key("top"), nil)
			app.chatModelSelect(c.Key("model"))
		})
	}, 720, 400)
	for _, label := range []string{locales.ModelProvider(), "Second Provider", locales.ModelID(), "alternate"} {
		if err := window.Click(label); err != nil {
			t.Fatalf("cannot select %q: %v", label, err)
		}
	}
	if got := app.currentModelChoice(); got != (modelChoice{ProviderID: secondID, Model: "alternate"}) {
		t.Fatalf("picked the wrong provider or model: %+v", got)
	}

	reopened := &chatApp{store: store}
	reopened.loadSettings()
	if reopened.currentModelChoice() != app.currentModelChoice() || reopened.settings.MaxTokens == nil || *reopened.settings.MaxTokens != 64 {
		t.Fatal("the selected provider, model, or generation settings did not survive reload")
	}
	assertRequest := func(backend *Chat, model, key string) {
		t.Helper()
		err := backend.stream(t.Context(), []message{{Role: "user", Content: "ping"}}, func(replyDelta) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		got := <-requests
		if got.model != model || got.auth != "Bearer "+key {
			t.Fatalf("wrong request model or credentials for %s", model)
		}
	}
	assertRequest(reopened.backend, "alternate", "second-key")
	legacyBackend, err := reopened.backendFor(legacyChoice)
	if err != nil {
		t.Fatal(err)
	}
	assertRequest(legacyBackend, "legacy-model", "legacy-key")
	reopened.selectProvider(modelChoice{ProviderID: legacyChoice.ProviderID})
	reopened.selectProvider(modelChoice{ProviderID: secondID})
	if reopened.settings.Model != "primary" {
		t.Fatal("switching providers did not select its default model")
	}
}
