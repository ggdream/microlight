package chat

import (
	"unicode/utf8"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/buildinfo"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
)

func (app *chatApp) settingsPage(c *ui.Context) {
	settings.Page(c, icons, app.settingsTab, func() {
		if !app.deferSettingsLeave(app.closeSettings) {
			app.closeSettings()
		}
	}, app.selectSettingsTab, func(c *ui.Context, tab settings.Tab) {
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
	app.settingsLeaveDialog(c.Key("unsaved-settings"))
}

func (app *chatApp) selectSettingsTab(tab settings.Tab) {
	if app.settingsTab == tab || app.deferSettingsLeave(func() { app.selectSettingsTab(tab) }) {
		return
	}
	if tab == settings.Skills {
		app.refreshSkills()
	}
	app.settingsTab = tab
}

func (app *chatApp) settingsLeaveDialog(c *ui.Context) {
	next := app.form.leave
	if next == nil {
		return
	}
	title, body := locales.UnsavedSettingsTitle(), locales.UnsavedSettingsBody()
	ui.Overlay(c, func() {
		backdrop := ui.Box(c.Key("backdrop")).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Center().Background(ui.RGBA(0, 0, 0, .4)).Modal()
		cancel, discard := backdrop.OverlayShortcut(0, ui.KeyEscape), false
		backdrop.Children(func() {
			ui.Column(c).Role(ui.RoleAlertDialog).Label(title).Description(body).
				Width(340).MaxWidthPercent(90).Padding(20).Gap(12).Radius(10).Background(white).
				Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .3)).Children(func() {
				ui.Text(c, title).FontSize(16).FontWeight(680)
				ui.Text(c, body).FontSize(13).TextColor(muted)
				ui.Row(c).Wrap().Justify(ui.End).Gap(8).Children(func() {
					stayLabel := locales.ContinueEditing()
					stay := pointerButton(c.Key("stay"), false).Label(stayLabel).AutoFocus().
						Height(34).PaddingX(13).AlignItems(ui.Center).Radius(7).Background(side).Border(1, border)
					if stay.Hovered() {
						stay.Background(hover)
					}
					stay.Children(func() { ui.Text(c, stayLabel).FontSize(13) })
					cancel = cancel || stay.Clicked()
					leaveLabel := locales.DiscardSettingsAndLeave()
					leave := pointerButton(c.Key("leave"), false).Label(leaveLabel).
						Height(34).PaddingX(13).AlignItems(ui.Center).Radius(7).Background(errorFG)
					if leave.Hovered() {
						leave.Background(errorFG.Alpha(.88))
					}
					leave.Children(func() { ui.Text(c, leaveLabel).FontSize(13).FontWeight(620).TextColor(white) })
					discard = leave.Clicked()
				})
			})
		})
		if cancel {
			app.form.leave = nil
		} else if discard {
			app.form.leave = nil
			app.discardSettingsChanges()
			next()
		}
	})
}

func (app *chatApp) generalSettings(c *ui.Context) {
	settings.Section(c, locales.SettingsGeneral(), locales.SettingsDescription(), func() {
		app.languageSettings(c)
		app.busySendSettings(c.Key("busy-send"))
		settings.Proxy(c, icons, &app.form.proxy, app.form.proxyError, app.saveProxySettings, app.openProxySettingsControl)
		if app.form.generalStatus.failed {
			ui.Text(c, app.form.generalStatus.message).FontSize(12).TextColor(theme.ErrorFG)
		}
		settings.Version(c.Key("version"), buildinfo.Version())
		settings.About(c.Key("about"), icons)
	})
}

func (app *chatApp) modelSettings(c *ui.Context) {
	form := &app.form
	savedKey := app.savedKeyForForm()
	var selectedProvider string
	if provider := app.providers.provider(form.providerID); provider != nil {
		selectedProvider = provider.Name
	}
	baseURLHint := defaultBaseURL
	if form.protocol == anthropicMessages {
		baseURLHint = "https://api.anthropic.com"
	}
	settings.ModelPanel(c, settings.ModelFields{
		BaseURL: &form.baseURL, APIKey: &form.apiKey, ProviderName: &form.providerName,
		SelectedProvider: selectedProvider,
		BaseURLHint:      baseURLHint,
		Protocol:         app.providerProtocolControl,
		Temperature:      &form.temperature, TopP: &form.topP, MaxTokens: &form.maxTokens,
		ContextThreshold: &form.contextThreshold, DefaultContextThreshold: defaultContextThreshold,
		SavedKeyLength: utf8.RuneCountInString(savedKey), RemoveKey: form.removeKey,
		OnChange: app.clearConnectionTest, Provider: app.providerSettingsControl,
		ClearKey: app.removeSavedKey,
		Models:   app.providerModelsControl,
	})
	settings.Footer(c, form.settingsStatus.message, form.settingsStatus.failed, app.saveSettings)
	app.providerDeleteDialog(c.Key("delete-provider"))
}
