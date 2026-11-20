package settings

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
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
	Provider, ProviderAction, Protocol, Models                      func(*ui.Context)
	BaseURLHint                                                     string
}

type ConnectionTestState struct {
	Running bool
}

func ModelPanel(c *ui.Context, form ModelFields) {
	layout := settingsLayout(c)
	scrollProvider := layout.Compact() || layout.Height < 540
	providerSwitcher := func() {
		CardWithAction(c.Key("provider-switcher"), locales.ModelProviders(), form.ProviderAction, func() {
			form.Provider(c.Key("provider"))
		})
	}
	if !scrollProvider {
		ui.Column(c.Key("provider-frame")).FillWidth().Shrink(0).AlignItems(ui.Center).
			Background(theme.Canvas).Padding(layout.Y(22), layout.Gutter(22), layout.Y(16)).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(610).Children(providerSwitcher)
		})
	}
	details := c.Key("provider-details")
	section(details, func() {
		if scrollProvider {
			providerSwitcher()
		}
		ui.Column(details).Gap(settingGap(c, 5)).Children(func() {
			title := locales.NewProvider()
			if form.SelectedProvider != "" {
				title = locales.ConfigureProvider()
			}
			ui.Row(details).FillWidth().Wrap().AlignItems(ui.Center).Gap(settingGap(c, 10)).Children(func() {
				material3.Text(details, title, material3.HeadlineMedium)
				if name := form.SelectedProvider; name != "" {
					ui.Box(details.Key("provider-name")).MaxWidth(240).MinWidth(0).
						Padding(4, 11).Radius(12).Background(theme.Tint).Tooltip(name).Children(func() {
						ui.Text(details, name).SingleLine().Ellipsis("…").
							FontSize(viewport.Font(details, 15)).FontWeight(650).TextColor(theme.Accent)
					})
				}
			})
			ui.Text(details, locales.ProviderSetupHint()).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
		})
	}, func() {
		Card(c, locales.Connection(), func() {
			Field(c, locales.ProviderName(), "OpenAI", form.ProviderName)
			protocol := c.Key("protocol")
			ui.Column(protocol).FillWidth().Gap(settingGap(c, 8)).Children(func() {
				form.Protocol(protocol)
			})
			Field(c, locales.BaseURL(), form.BaseURLHint, form.BaseURL).
				Keyboard(ui.KeyboardURL).Capitalize(ui.CapitalizeNone).Autocorrect(false).OnChange(form.OnChange)
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
				theme.PointerButton(c, false).Label(locales.ClearSavedKey()).Padding(settingY(c, 4), settingX(c, 5)).AlignSelf(ui.Start).Children(func() {
					ui.Text(c, locales.ClearKey()).FontSize(viewport.Font(c, 11)).TextColor(theme.Muted)
				}).OnClick(form.ClearKey)
			}
		}).Gap(settingsLayout(c).Density(17, 10))
		form.Models(c.Key("models"))
		Card(c, locales.Generation(), func() {
			ui.Text(c, locales.GenerationHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
			ui.Row(c).Gap(settingGap(c, 10)).AlignItems(ui.Start).Children(func() {
				Field(c, locales.Temperature(), "0–2", form.Temperature)
				Field(c, "Top P", "0–1", form.TopP)
			})
			Field(c, locales.MaxOutputTokens(), locales.PositiveInteger(), form.MaxTokens)
			Field(c, locales.CompressionThreshold(), locales.CompressionDefault(form.DefaultContextThreshold), form.ContextThreshold)
			ui.Text(c, locales.CompressionHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
		})
	})
}

func ConnectionTest(c *ui.Context, state *ConnectionTestState, model string, run func()) {
	running := state != nil && state.Running
	ui.Row(c).AlignItems(ui.Center).Gap(settingGap(c, 10)).Children(func() {
		label := locales.TestConnection()
		if running {
			label = locales.TestingConnection()
		}
		button := material3.Button(c, label, material3.FilledButton, running || model == "").Height(settingsLayout(c).Target(34)).PaddingX(settingX(c, 14)).Gap(settingGap(c, 7))
		button.Children(func() {
			color := material3.Foreground(material3.FilledButton, running || model == "")
			if running {
				size := settingX(c, 14)
				ui.Spinner(c.Key("spinner")).Label(locales.TestingConnection()).Size(size, size).Shrink(0).TextColor(color)
			}
			ui.Text(c, label).FontSize(viewport.Font(c, 12)).FontWeight(620).TextColor(color)
		}).OnClick(run)
	})
}
