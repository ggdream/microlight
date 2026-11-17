package chat

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/pkg/websearch"
)

func TestWebSearchSettingsSaveProviderAndKey(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	app.loadSettings()
	app.openSettings()
	app.selectSettingsTab(settings.Tools)
	window := ui.NewTester(app.settingsPage, 970, 700)
	click := func(label string) {
		t.Helper()
		if err := window.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click(locales.EnableWebSearch())
	click("DuckDuckGo")
	for _, label := range []string{locales.SearchProviderAliyun(), locales.SearchProviderTencent(), locales.SearchProviderVolcengine()} {
		if _, ok := window.Find(label); !ok {
			t.Errorf("search provider %q is missing from the menu", label)
		}
	}
	click("DeepSeek")
	click(locales.SaveSettings())
	if app.settings.WebSearch.Enabled {
		t.Fatal("enabled a credentialed provider without an API key")
	}
	if _, ok := window.Find(locales.SearchKeyRequired()); !ok {
		t.Fatal("missing credential validation feedback")
	}
	click("DeepSeek " + locales.ApiKey())
	window.Type("test-search-secret")
	click(locales.SaveSettings())
	if !app.settings.WebSearch.Enabled || app.settings.WebSearch.Selected() != websearch.DeepSeek ||
		app.settings.WebSearch.Options[websearch.DeepSeek].APIKey != "test-search-secret" {
		t.Fatal("search settings were not saved")
	}
	if app.extensionsForm.search.HasChanges(app.settings.WebSearch) {
		t.Fatal("search settings remained dirty after saving")
	}
	reloaded := &chatApp{store: store}
	reloaded.loadSettings()
	if !reloaded.settings.WebSearch.Enabled || reloaded.settings.WebSearch.Options[websearch.DeepSeek].APIKey != "test-search-secret" {
		t.Fatal("search settings did not persist across reload")
	}
}
