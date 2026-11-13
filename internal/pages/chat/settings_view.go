package chat

import (
	"unicode/utf8"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
)

func (app *chatApp) settingsPage(c *ui.Context) {
	settings.Page(c, icons, app.settingsTab, app.closeSettings, func(tab settings.Tab) {
		if app.settingsTab != tab && tab == settings.Skills {
			app.refreshSkills()
		}
		app.settingsTab = tab
	}, func(c *ui.Context, tab settings.Tab) {
		switch tab {
		case settings.Model:
			app.modelSettings(c.Key("model-content"))
		case settings.Skills:
			app.skillsSettings(c.Key("skills-content"))
		case settings.Tools:
			app.mcpSettings(c.Key("mcp-content"))
		case settings.Archive:
			app.archiveSettings(c.Key("archive-content"))
		default:
			app.generalSettings(c.Key("general-content"))
		}
	})
}

func (app *chatApp) generalSettings(c *ui.Context) {
	settings.Section(c, locales.SettingsGeneral(), locales.SettingsDescription(), func() {
		app.languageSettings(c)
		app.busySendSettings(c.Key("busy-send"))
		settings.Proxy(c, icons, &app.form.proxy, app.openProxySettingsControl)
	})
	settings.Footer(c, app.form.generalStatus.message, app.form.generalStatus.failed, app.saveProxySettings)
}

func (app *chatApp) modelSettings(c *ui.Context) {
	form := &app.form
	savedKey := app.savedKeyForForm()
	settings.ModelPanel(c, settings.ModelFields{
		BaseURL: &form.baseURL, APIKey: &form.apiKey,
		Temperature: &form.temperature, TopP: &form.topP, MaxTokens: &form.maxTokens,
		ContextThreshold: &form.contextThreshold, DefaultContextThreshold: defaultContextThreshold,
		SavedKeyLength: utf8.RuneCountInString(savedKey), RemoveKey: form.removeKey,
		OnChange: app.clearConnectionTest, Provider: app.providerSettingsControl, Models: app.providerModelsControl,
		ConnectionTest: func(c *ui.Context) {
			var state *settings.ConnectionTestState
			if test := form.test; test != nil {
				state = &settings.ConnectionTestState{Running: test.cancel != nil, Failed: test.failed, Message: test.message}
			}
			settings.ConnectionTest(c, state, savedKey != "", app.testConnection, app.removeSavedKey)
		},
	})
	settings.Footer(c, form.settingsStatus.message, form.settingsStatus.failed, app.saveSettings)
}
