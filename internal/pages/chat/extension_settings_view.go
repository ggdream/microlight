package chat

import (
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
)

type extensionsForm struct {
	mcp                       settings.MCPForm
	search                    settings.WebSearchForm
	ocr                       settings.OCRForm
	asr                       settings.ASRForm
	skillIndex                *skillIndex
	skillQuery, expandedSkill string
	browserError              string
}

func (app *chatApp) toolsSettings(c *ui.Context) {
	settings.Section(c, locales.SettingsTools(), locales.ToolsSettingsHint(), func() {
		app.extensionsForm.search.View(c.Key("web-search"), icons, app.settings.WebSearch, func(search localtools.SearchConfig) error {
			config := app.settings
			config.WebSearch = search
			return app.applySettings(config)
		})
		app.extensionsForm.ocr.View(c.Key("ocr"), icons, app.settings.OCR, func(ocr localtools.OCRConfig) error {
			config := app.settings
			config.OCR = ocr
			return app.applySettings(config)
		})
		app.extensionsForm.asr.View(c.Key("asr"), icons, app.settings.ASR, func(asr localtools.ASRConfig) error {
			config := app.settings
			config.ASR = asr
			return app.applySettings(config)
		})
		if desktop.Desktop {
			// Desktop-only browser automation follows the portable built-in tools.
			app.browserSettings(c.Key("browser-use"))
		}
		app.extensionsForm.mcp.View(c.Key("mcp"), icons, app.settings.MCPServers, func(servers []mcptools.Server) error {
			config := app.settings
			config.MCPServers = servers
			return app.applySettings(config)
		}, func(next func()) {
			if !app.extensionsForm.mcp.HasChanges(app.settings.MCPServers) || !app.deferSettingsLeave(next) {
				next()
			}
		})
	})
}

func (app *chatApp) browserSettings(c *ui.Context) {
	enabled := app.settings.BrowserUse
	mode := app.settings.BrowserMode.Effective()
	save := func() {
		config := app.settings
		config.BrowserUse, config.BrowserMode = enabled, mode
		app.extensionsForm.browserError = ""
		if err := app.applySettings(config); err != nil {
			app.extensionsForm.browserError = err.Error()
		}
	}
	settings.Card(c, locales.BrowserUseTool(), func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, locales.BrowserUseHint()).Grow(1).MinWidth(0).FontSize(viewport.Font(c, 12)).TextColor(muted)
			theme.Switch(c.Key("enabled"), &enabled, locales.EnableBrowserUse(), save)
		})
		settings.BrowserModeSelect(c.Key("mode"), icons, &mode, save)
		if app.extensionsForm.browserError != "" {
			ui.Text(c, app.extensionsForm.browserError).FontSize(viewport.Font(c, 12)).TextColor(errorFG)
		}
	})
}
