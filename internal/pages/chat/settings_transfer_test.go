package chat

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/pkg/ocr"
	"github.com/ggdream/microlight/pkg/proxy"
	"github.com/ggdream/microlight/pkg/websearch"
)

func TestSettingsTransferExportsDisabledWebSearchAndReadsOlderFiles(t *testing.T) {
	app := &chatApp{language: settings.SystemLanguage, busySend: busySendQueue}
	data, err := app.exportSettings()
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	var search struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(fields["web_search"], &search); err != nil || search.Enabled == nil || *search.Enabled {
		t.Fatal("export must explicitly include disabled web search")
	}
	delete(fields, "web_search")
	delete(fields, "browser_mode")
	legacy, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	_, config, err := readSettingsFile(bytes.NewReader(legacy))
	if err != nil || config.WebSearch.Enabled || config.BrowserMode.Effective() != localtools.BrowserForeground {
		t.Fatalf("expected older files to disable search and default to foreground browsing: %v", err)
	}
}

func TestSettingsTransferPreservesSecretsAndKeepsConversationData(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{
		store: store, language: settings.Chinese, busySend: busySendSteer,
		systemPrompt: "Test instruction", desktop: desktopPreferences{Shortcut: "CmdOrCtrl+Shift+Space", Notification: notifyAction, HideStatistics: true},
		settings: modelSettings{
			BrowserUse: true, BrowserMode: localtools.BrowserBackground,
			Proxy: proxy.Config{Mode: proxy.Custom, URL: "http://user:proxy-test-key@localhost:8080"},
			WebSearch: localtools.SearchConfig{
				Enabled: true, Provider: websearch.DeepSeek,
				Options: map[websearch.ID]websearch.Options{
					websearch.DeepSeek:   {APIKey: "search-test-key"},
					websearch.Baidu:      {APIKey: "inactive-search-test-key"},
					websearch.Aliyun:     {APIKey: "aliyun-test-key"},
					websearch.Tencent:    {APIKey: "tencent-test-key"},
					websearch.Volcengine: {APIKey: "volcengine-test-key"},
					websearch.SearXNG:    {BaseURL: "https://search.example.test"},
					websearch.Google:     {APIKey: "google-test-key", SearchEngineID: "test-cx"},
					websearch.Tavily:     {APIKey: "tavily-test-key"},
					websearch.Firecrawl:  {APIKey: "firecrawl-test-key"},
				},
			},
			OCR: localtools.OCRConfig{Enabled: true, Provider: localtools.PaddleOCR, Token: "ocr-test-token", TextModel: ocr.PPV5Latin, DocumentModel: ocr.StructureV3},
			MCPServers: []mcptools.Server{{
				Name: "test_mcp", Transport: mcptools.HTTP, URL: "https://example.test/mcp", TimeoutSeconds: 300,
				Headers: []mcptools.Header{{Name: "Authorization", Value: "Bearer mcp-test-key"}},
			}},
		},
	}
	app.providers = providerCatalog{
		Providers: []modelProvider{{
			ID: "test-provider", Name: "Test Provider", Models: []string{"default", "alternate"},
			modelSettings: modelSettings{
				BaseURL: "https://example.test/v1", Model: "default", APIKey: "model-test-key", Protocol: openAIChat,
				Workspace: filepath.Join(t.TempDir(), "workspace"),
			},
		}},
		Selected: modelChoice{ProviderID: "test-provider", Model: "alternate"},
	}
	chat := app.newConversation("private-chat-title", localtools.Ask, t.TempDir())
	chat.Turns = []*turn{userTurn("private-chat-text")}
	chat.Turns[0].Attachments = []*attachment.File{{Name: "private-attachment.txt", Data: []byte("private-file-content")}}
	app.save(chat)
	app.draft.SetText("private-draft")
	if !app.persistDraft() {
		t.Fatal("could not seed the draft")
	}
	data, err := app.exportSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"workspace", "private-chat-title", "private-chat-text", "private-attachment.txt", "private-draft"} {
		if bytes.Contains(data, []byte(private)) {
			t.Fatal("export included conversation data")
		}
	}
	file, config, err := readSettingsFile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if config.Model != "alternate" || file.Models.Providers[0].Model != "default" ||
		config.APIKey != "model-test-key" || config.Proxy != app.settings.Proxy ||
		!reflect.DeepEqual(config.MCPServers, app.settings.MCPServers) ||
		!reflect.DeepEqual(config.WebSearch, app.settings.WebSearch) || config.OCR != app.settings.OCR || !config.BrowserUse || config.BrowserMode != localtools.BrowserBackground {
		t.Fatal("provider, proxy, model, or built-in tool settings failed to round-trip")
	}
	for _, bad := range []string{string(data) + " null", strings.Replace(string(data), `"version": 1`, `"version": 42`, 1)} {
		if _, _, err := readSettingsFile(strings.NewReader(bad)); err == nil {
			t.Fatal("accepted an invalid settings file")
		}
	}
	if _, err := store.Exec(`CREATE TRIGGER reject_import BEFORE INSERT ON config
		WHEN NEW.key = 'desktop_preferences' BEGIN SELECT RAISE(ABORT, 'test rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.importSettings(file, config); err == nil {
		t.Fatal("expected a failed import")
	}
	if saved, err := store.getConfig(providersConfigKey); err != nil || saved != "" {
		t.Fatal("failed import partially wrote the configuration")
	}
	if _, err := store.Exec("DROP TRIGGER reject_import"); err != nil {
		t.Fatal(err)
	}
	destination := &chatApp{store: store}
	if err := destination.applySettingsImport(file, config); err != nil {
		t.Fatal(err)
	}
	if destination.settings.APIKey != config.APIKey || destination.settings.OCR != config.OCR || destination.providers.Selected != app.providers.Selected ||
		destination.language != app.language || destination.systemPrompt != app.systemPrompt {
		t.Fatal("the app did not apply the imported settings")
	}
	reloaded := &chatApp{store: store}
	reloaded.loadSettings()
	reloaded.loadBusySendMode()
	reloaded.loadSystemPrompt()
	reloaded.loadDesktopPreferences()
	reloaded.loadHistory()
	if reloaded.settings.APIKey != config.APIKey || reloaded.settings.Proxy != config.Proxy ||
		!reflect.DeepEqual(reloaded.settings.MCPServers, config.MCPServers) ||
		!reflect.DeepEqual(reloaded.settings.WebSearch, config.WebSearch) || reloaded.settings.OCR != config.OCR || !reloaded.settings.BrowserUse || reloaded.settings.BrowserMode != config.BrowserMode ||
		reloaded.busySend != app.busySend ||
		reloaded.systemPrompt != app.systemPrompt || reloaded.desktop != app.desktop {
		t.Fatal("imported settings did not persist")
	}
	if language, err := store.getConfig("language"); err != nil || language != string(app.language) {
		t.Fatal("language was not imported")
	}
	if len(reloaded.history.Chats) != 1 || reloaded.history.Selected != chat.ID ||
		reloaded.history.Chats[0].Turns[0].Content != "private-chat-text" ||
		string(reloaded.history.Chats[0].Turns[0].Attachments[0].Data) != "private-file-content" ||
		!reloaded.restoreDraft(chat.ID) || reloaded.draft.Markdown() != "private-draft" {
		t.Fatal("import modified the existing conversation, attachments, or draft")
	}
}
