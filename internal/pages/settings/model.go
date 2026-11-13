package settings

import (
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ModelFields struct {
	BaseURL, APIKey, Temperature, TopP, MaxTokens, ContextThreshold *string
	ProviderName                                                    *string
	SelectedProvider                                                string
	SavedKeyLength                                                  int
	RemoveKey                                                       bool
	DefaultContextThreshold                                         int
	OnChange                                                        func()
	ClearKey                                                        func()
	Provider, Protocol, Models                                      func(*ui.Context)
	BaseURLHint                                                     string
}

type ConnectionTestState struct {
	Running, Failed bool
	Message         string
}

func ModelPanel(c *ui.Context, form ModelFields) {
	ui.Column(c.Key("provider-switcher")).FillWidth().Shrink(0).AlignItems(ui.Center).
		Background(theme.Canvas).Padding(22, 22, 16).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(610).Children(func() {
			Card(c, locales.ModelProviders(), func() {
				form.Provider(c.Key("provider"))
			})
		})
	})
	details := c.Key("provider-details")
	section(details, func() {
		ui.Column(details).Gap(5).Children(func() {
			title := locales.NewProvider()
			if form.SelectedProvider != "" {
				title = locales.ConfigureProvider()
			}
			ui.Row(details).FillWidth().Wrap().AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(details, title).FontSize(29).FontWeight(680)
				if name := form.SelectedProvider; name != "" {
					ui.Box(details.Key("provider-name")).MaxWidth(240).MinWidth(0).
						Padding(4, 11).Radius(12).Background(theme.Tint).Tooltip(name).Children(func() {
						ui.Text(details, name).SingleLine().Ellipsis("…").
							FontSize(15).FontWeight(650).TextColor(theme.Accent)
					})
				}
			})
			ui.Text(details, locales.ProviderSetupHint()).FontSize(13).TextColor(theme.Muted)
		})
	}, func() {
		Card(c, locales.Connection(), func() {
			Field(c, locales.ProviderName(), "OpenAI", form.ProviderName)
			protocol := c.Key("protocol")
			ui.Column(protocol).FillWidth().Gap(8).Children(func() {
				form.Protocol(protocol)
			})
			Field(c, locales.BaseURL(), form.BaseURLHint, form.BaseURL).OnChange(form.OnChange)
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
				theme.PointerButton(c, false).Label(locales.ClearSavedKey()).Padding(4, 5).AlignSelf(ui.Start).Children(func() {
					ui.Text(c, locales.ClearKey()).FontSize(11).TextColor(theme.Muted)
				}).OnClick(form.ClearKey)
			}
		}).Gap(17)
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

func ConnectionTest(c *ui.Context, state *ConnectionTestState, model string, run func()) {
	running := state != nil && state.Running
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		button := theme.PointerButton(c, running || model == "").Label(locales.TestConnection()).Height(34).Shrink(0).PaddingX(12).
			Radius(7).Border(1, theme.Border).AlignItems(ui.Center)
		if button.Hovered() && !running && model != "" {
			button.Background(theme.Tint)
		}
		button.Children(func() {
			color := theme.Accent
			if model == "" {
				color = theme.Muted
			}
			ui.Text(c, locales.TestConnection()).FontSize(12).FontWeight(620).TextColor(color)
		}).OnClick(run)
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
