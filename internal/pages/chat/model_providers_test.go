package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
)

func TestProviderChipDeletionRequiresConfirmation(t *testing.T) {
	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	app.loadSettings()
	app.openSettings()
	app.settingsTab = settings.Model
	provider := app.providers.Providers[0]
	window := ui.NewTester(app.settingsPage, 1000, 750)
	remove := locales.RemoveProvider(provider.Name)
	openDelete := func() {
		t.Helper()
		chip, ok := window.Find(provider.Name)
		if !ok {
			t.Fatal("provider chip is missing")
		}
		window.Move(chip.X+chip.W/2, chip.Y+chip.H/2)
		window.Advance(200 * time.Millisecond)
		if err := window.Click(remove); err != nil {
			t.Fatal(err)
		}
		if !window.HasText(locales.DeleteProviderTitle()) || len(app.providers.Providers) != 1 {
			t.Fatal("clicking remove should open confirmation without deleting the provider")
		}
	}
	if _, ok := window.Find(remove); ok {
		t.Fatal("remove should be hidden before hover")
	}
	openDelete()
	if err := window.Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	openDelete()
	if err := window.Click(locales.DeleteProvider()); err != nil {
		t.Fatal(err)
	}
	reopened := &chatApp{store: store}
	reopened.loadSettings()
	if len(reopened.providers.Providers) != 0 || reopened.backend.configErr == nil {
		t.Fatal("deleting the last provider should persist an empty catalog")
	}
}

func TestModelRowsKeepOrderAndSeparateDefaultFromTestSelection(t *testing.T) {
	app := &chatApp{}
	app.form.modelForm = app.providerForm("")
	*app.form.defaultModel = "primary"
	window := ui.NewTester(app.providerModelsControl, 610, 550)
	click := func(label string) {
		t.Helper()
		if err := window.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click(locales.AddProviderModel())
	if len(app.form.models) != 2 || *app.form.models[1] != "" {
		t.Fatal("add should create an editable model row immediately")
	}

	// Rebuild after updating the second row so the radio and picker use its ID.
	*app.form.models[1] = "alternate"
	window = ui.NewTester(app.providerModelsControl, 610, 550)
	click(locales.ChooseDefaultModel("alternate"))
	if app.connectionConfig().Model != "alternate" || *app.form.models[0] != "primary" {
		t.Fatal("selecting the default should preserve model order")
	}
	click("alternate")
	click("primary")
	if app.form.testModel != "primary" || app.connectionConfig().Model != "alternate" {
		t.Fatal("the test selection should be independent of the default model")
	}
}

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
	legacy := modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "legacy-model", APIKey: "legacy-key"}
	if err := store.saveSettings(legacy); err != nil {
		t.Fatal(err)
	}
	app := &chatApp{store: store}
	app.loadSettings()
	legacyChoice := app.currentModelChoice()
	app.openSettings()
	if app.savedKeyForForm() != "legacy-key" || app.form.defaultModelID() != "legacy-model" {
		t.Fatal("legacy settings were not migrated into the first provider")
	}

	app.editProvider("")
	app.form.providerName, app.form.baseURL = "Second Provider", server.URL
	app.form.protocol = openAIChat
	*app.form.defaultModel = "primary"
	app.form.apiKey, app.form.maxTokens = "second-key", "64"
	alternate := "alternate"
	app.form.models = append(app.form.models, &alternate)
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
