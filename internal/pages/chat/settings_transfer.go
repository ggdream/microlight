package chat

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/pkg/proxy"
)

const settingsFileVersion = 1

type settingsFile struct {
	Version      int                         `json:"version"`
	Models       *providerCatalog            `json:"models"`
	Proxy        proxy.Config                `json:"proxy"`
	MCPServers   []mcptools.Server           `json:"mcp_servers"`
	WebSearch    localtools.SearchConfig     `json:"web_search"`
	OCR          localtools.OCRConfig        `json:"ocr,omitzero"`
	BrowserUse   bool                        `json:"browser_use"`
	BrowserMode  localtools.BrowserMode      `json:"browser_mode"`
	Language     settings.LanguagePreference `json:"language"`
	BusySend     busySendMode                `json:"busy_send"`
	SystemPrompt string                      `json:"system_prompt"`
	Desktop      desktopPreferences          `json:"desktop"`
}

func (app *chatApp) exportSettings() ([]byte, error) {
	catalog := app.providers
	catalog.Providers = slices.Clone(catalog.Providers)
	for i := range catalog.Providers {
		provider := &catalog.Providers[i]
		provider.MCPServers = nil
		provider.WebSearch = localtools.SearchConfig{}
		provider.OCR = localtools.OCRConfig{}
		provider.BrowserUse, provider.BrowserMode = false, ""
		provider.AccessMode, provider.Workspace = "", ""
	}
	return json.MarshalIndent(settingsFile{
		Version: settingsFileVersion, Models: &catalog, Proxy: app.settings.Proxy,
		MCPServers: app.settings.MCPServers, Language: app.language, BusySend: app.busySend,
		WebSearch: app.settings.WebSearch, OCR: app.settings.OCR, BrowserUse: app.settings.BrowserUse, BrowserMode: app.settings.BrowserMode.Effective(),
		SystemPrompt: app.systemPrompt, Desktop: app.desktop,
	}, "", "  ")
}

func readSettingsFile(reader io.Reader) (settingsFile, modelSettings, error) {
	var file settingsFile
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return file, modelSettings{}, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return file, modelSettings{}, errors.New("unexpected data after settings")
	}
	config, err := file.validate()
	return file, config, err
}

func (file *settingsFile) validate() (modelSettings, error) {
	invalid := errors.New("invalid Microlight settings")
	if file.Version != settingsFileVersion || file.Models == nil {
		return modelSettings{}, invalid
	}
	switch file.Language {
	case settings.SystemLanguage, settings.English, settings.Chinese:
	default:
		return modelSettings{}, invalid
	}
	if (file.BusySend != busySendQueue && file.BusySend != busySendSteer) ||
		file.Desktop.Notification != clampNotificationMode(file.Desktop.Notification) {
		return modelSettings{}, invalid
	}
	ids, names := make(map[string]bool), make(map[string]bool)
	for i := range file.Models.Providers {
		provider := &file.Models.Providers[i]
		provider.ID, provider.Name = strings.TrimSpace(provider.ID), strings.TrimSpace(provider.Name)
		name := strings.ToLower(provider.Name)
		if provider.ID == "" || name == "" || ids[provider.ID] || names[name] {
			return modelSettings{}, invalid
		}
		ids[provider.ID], names[name] = true, true
		models := make(map[string]bool)
		for j, model := range provider.Models {
			model = strings.TrimSpace(model)
			if model == "" || models[model] {
				return modelSettings{}, invalid
			}
			models[model], provider.Models[j] = true, model
		}
		provider.MCPServers = nil
		provider.WebSearch = localtools.SearchConfig{}
		provider.OCR = localtools.OCRConfig{}
		provider.BrowserUse, provider.BrowserMode = false, ""
		provider.AccessMode, provider.Workspace = "", ""
		if err := provider.modelSettings.validate(); err != nil {
			return modelSettings{}, err
		}
		provider.AccessMode = ""
		if !models[provider.Model] {
			return modelSettings{}, invalid
		}
	}
	if len(file.Models.Providers) != 0 {
		selected := file.Models.Selected
		provider := file.Models.provider(selected.ProviderID)
		if provider == nil || (selected.Model != "" && !slices.Contains(provider.Models, selected.Model)) {
			return modelSettings{}, invalid
		}
	} else if file.Models.Selected != (modelChoice{}) {
		return modelSettings{}, invalid
	} else {
		file.Models.Providers = []modelProvider{}
	}
	shared := modelSettings{BaseURL: defaultBaseURL, Model: defaultModel, Proxy: file.Proxy, MCPServers: file.MCPServers, WebSearch: file.WebSearch, OCR: file.OCR, BrowserUse: file.BrowserUse, BrowserMode: file.BrowserMode}
	config := file.Models.activeConfig(shared)
	err := config.validate()
	return config, err
}

func (store *localStore) importSettings(file settingsFile, config modelSettings) error {
	desktop, err := json.Marshal(file.Desktop)
	if err != nil {
		return err
	}
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := writeConfiguration(tx, config, file.Models); err != nil {
		return err
	}
	for key, value := range map[string]string{
		"language": string(file.Language), busySendConfigKey: string(file.BusySend),
		systemPromptConfigKey: file.SystemPrompt, desktopConfigKey: string(desktop),
	} {
		if err := storage.PutConfig(tx, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
