package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"unicode/utf8"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

func (app *chatApp) settingsPage(c *ui.Context) {
	back := func() {
		if !app.deferSettingsLeave(app.closeSettings) {
			app.closeSettings()
		}
	}
	if windowLayout(c).Compact() && app.transferLoading == "" {
		// Android's system Back is delivered as Escape by MyGo. Open menus
		// and dialogs take it first; the settings page then returns to chat.
		c.OnWindowShortcut(0, ui.KeyEscape, back)
	}
	settings.Page(c, icons, app.settingsTab, back, app.selectSettingsTab, func(c *ui.Context, tab settings.Tab) {
		switch tab {
		case settings.Model:
			app.modelSettings(c.Key("model-content"))
		case settings.Skills:
			app.skillsSettings(c.Key("skills-content"))
		case settings.Tools:
			app.toolsSettings(c.Key("tools-content"))
		case settings.Archive:
			app.archiveSettings(c.Key("archive-content"))
		case settings.About:
			settings.AboutPanel(c.Key("about-content"), icons, app.versionSettings)
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
	app.shortcutRecording = false
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
		insetDialog(c, backdrop)
		cancel, discard := backdrop.OverlayShortcut(0, ui.KeyEscape), false
		backdrop.Children(func() {
			panel := material3.Elevate(ui.Column(c).Role(ui.RoleAlertDialog).Label(title).Description(body).
				Width(340).MaxWidthPercent(90).MaxHeight(dialogMaxHeight(c)).Padding(windowLayout(c).Density(20, 14)).Gap(windowLayout(c).Density(12, 8)).Radius(10).Background(white), 3)

			mobileDialog(c, backdrop, panel)
			panel.Children(func() {
				dialogBody(c, func() {
					ui.Text(c, title).FontSize(viewport.Font(c, 16)).FontWeight(680)
					ui.Text(c, body).FontSize(viewport.Font(c, 13)).TextColor(muted)
				})
				dialogActions(c).Children(func() {
					stayLabel := locales.ContinueEditing()
					stay := material3.ButtonStyle(pointerButton(c.Key("stay"), false).Label(stayLabel).AutoFocus().
						Height(34).PaddingX(13).AlignItems(ui.Center), material3.TextButton, false)
					if stay.Hovered() {
						stay.Background(hover)
					}
					stay.Children(func() { ui.Text(c, stayLabel).FontSize(viewport.Font(c, 13)) })
					cancel = cancel || stay.Clicked()
					leaveLabel := locales.DiscardSettingsAndLeave()
					leave := pointerButton(c.Key("leave"), false).Label(leaveLabel).
						Height(34).PaddingX(13).AlignItems(ui.Center).Radius(material3.ShapeSmall).Background(errorFG)
					if leave.Hovered() {
						leave.Background(errorFG.Alpha(.88))
					}
					leave.Children(func() { ui.Text(c, leaveLabel).FontSize(viewport.Font(c, 13)).FontWeight(620).TextColor(white) })
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
		app.desktopSettings(c.Key("desktop"))
		app.statisticsSettings(c.Key("statistics"))
		if !desktop.Web {
			settings.Proxy(c, icons, &app.form.proxy, app.form.proxyError, app.saveProxySettings, app.openProxySettingsControl)
		}
		app.systemPromptSettings(c.Key("system-prompt"))
		app.settingsTransfer(c.Key("transfer"))
		if app.form.generalStatus.failed {
			ui.Text(c, app.form.generalStatus.message).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
		}
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
