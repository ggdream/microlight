package settings

import (
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ModelFields struct {
	BaseURL, APIKey, Temperature, TopP, MaxTokens, ContextThreshold *string
	SavedKeyLength                                                  int
	RemoveKey                                                       bool
	DefaultContextThreshold                                         int
	OnChange                                                        func()
	Provider, Models, ConnectionTest                                func(*ui.Context)
}

type ConnectionTestState struct {
	Running, Failed bool
	Message         string
}

func ModelPanel(c *ui.Context, form ModelFields) {
	Section(c, locales.SettingsModel(), locales.SettingsModelHint(), func() {
		form.Provider(c.Key("provider"))
		Card(c, locales.Connection(), func() {
			Field(c, locales.BaseURL(), "https://api.openai.com/v1", form.BaseURL).OnChange(form.OnChange)
			placeholder := locales.ApiKeyHint()
			hasSavedKey := form.SavedKeyLength > 0
			if hasSavedKey {
				placeholder = strings.Repeat("•", form.SavedKeyLength)
			} else if form.RemoveKey {
				placeholder = locales.RemoveKeyHint()
			}
			keyInput := Field(c, locales.ApiKey(), placeholder, form.APIKey).
				Password().OnChange(form.OnChange)
			if hasSavedKey {
				keyInput.PlaceholderColor(theme.Ink).Label(locales.ApiKeyConfigured())
			}
			form.ConnectionTest(c)
		})
		form.Models(c.Key("models"))
		Card(c, locales.Generation(), func() {
			ui.Text(c, locales.GenerationHint()).FontSize(12).TextColor(theme.Muted)
			ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
				Field(c, locales.Temperature(), "0–2", form.Temperature)
				Field(c, "Top P", "0–1", form.TopP)
			})
			Field(c, locales.MaxOutputTokens(), locales.PositiveInteger(), form.MaxTokens)
			Field(c, locales.CompressionThreshold(), locales.CompressionDefault(form.DefaultContextThreshold), form.ContextThreshold)
			ui.Text(c, locales.CompressionHint()).FontSize(12).TextColor(theme.Muted)
		})
	})
}

func ConnectionTest(c *ui.Context, state *ConnectionTestState, hasSavedKey bool, run, clearKey func()) {
	running := state != nil && state.Running
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		button := theme.PointerButton(c, running).Label(locales.TestConnection()).Height(34).Shrink(0).PaddingX(12).
			Radius(7).Border(1, theme.Border).AlignItems(ui.Center)
		if button.Hovered() && !running {
			button.Background(theme.Tint)
		}
		button.Children(func() {
			ui.Text(c, locales.TestConnection()).FontSize(12).FontWeight(620).TextColor(theme.Accent)
		}).OnClick(run)
		if hasSavedKey {
			theme.PointerButton(c, false).Label(locales.ClearSavedKey()).Padding(4, 5).Shrink(0).Children(func() {
				ui.Text(c, locales.ClearKey()).FontSize(11).TextColor(theme.Muted)
			}).OnClick(clearKey)
		}
		if state != nil {
			color := theme.Accent
			if state.Failed {
				color = theme.ErrorFG
			} else if running {
				color = theme.Muted
			}
			ui.Text(c, state.Message).Grow(1).MinWidth(0).FontSize(12).TextColor(color)
		}
	})
}
