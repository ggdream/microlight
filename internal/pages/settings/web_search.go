package settings

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"maps"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/theme"
	search "github.com/ggdream/microlight/pkg/websearch"

	"github.com/egoist/mygo/ui"
)

type WebSearchForm struct {
	draft *localtools.SearchConfig
	err   string
}

func (form WebSearchForm) HasChanges(saved localtools.SearchConfig) bool {
	return form.draft != nil && (form.draft.Enabled != saved.Enabled ||
		form.draft.Selected() != saved.Selected() || !maps.Equal(form.draft.Options, saved.Options))
}

func (form *WebSearchForm) View(c *ui.Context, icons asseticon.Set, saved localtools.SearchConfig, save func(localtools.SearchConfig) error) {
	if form.draft == nil {
		draft := saved
		draft.Provider = saved.Selected()
		draft.Options = maps.Clone(saved.Options)
		if draft.Options == nil {
			draft.Options = make(map[search.ID]search.Options)
		}
		form.draft = &draft
	}
	draft := form.draft
	providerID := draft.Provider
	options := draft.Options[providerID]
	updateOptions := func() {
		if options == (search.Options{}) {
			delete(draft.Options, providerID)
		} else {
			draft.Options[providerID] = options
		}
		form.err = ""
	}
	ui.Column(c).FillWidth().Gap(settingGap(c, 12)).Children(func() {
		ui.Text(c, locales.BuiltInTools()).FontSize(viewport.Font(c, 18)).FontWeight(660)
		ui.Column(c).FillWidth().Background(theme.White).Radius(13).Border(1, theme.Border).
			Padding(settingY(c, 15), settingX(c, 16)).Gap(settingGap(c, 11)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(settingsLayout(c).Density(12, 8)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(settingGap(c, 4)).Children(func() {
					ui.Text(c, locales.WebSearch()).FontSize(viewport.Font(c, 15)).FontWeight(660)
					ui.Text(c, locales.WebSearchHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
				})
				theme.Switch(c.Key("enabled"), &draft.Enabled, locales.EnableWebSearch(), func() { form.err = "" })
			})
			definition, _ := search.Lookup(providerID)
			settingsSelect(c.Key("provider"), icons, locales.SearchProvider(), &draft.Provider, search.IDs(),
				searchProviderLabel, func() { form.err = "" })
			// Key the inputs by provider to keep editing state tied to the selected service.
			fields := c.Key(string(definition.ID))
			if definition.APIKey {
				Field(fields, locales.ApiKey(), "", &options.APIKey).
					Label(searchProviderLabel(providerID) + " " + locales.ApiKey()).Password().OnChange(updateOptions)
			}
			if definition.SearchEngineID {
				Field(fields, locales.SearchEngineId(), "", &options.SearchEngineID).OnChange(updateOptions)
				ui.Text(fields, locales.GoogleSearchAvailability()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
			}
			if providerID == search.Wikipedia {
				ui.Text(fields, locales.WikipediaSearchScope()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
			}
			if definition.BaseURL {
				Field(fields, "Base URL", "http://localhost:8888", &options.BaseURL).OnChange(updateOptions)
				ui.Text(fields, locales.SearxngJSONHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
			}
			if form.err != "" {
				ui.Text(c, form.err).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
			}
			if form.HasChanges(saved) {
				ui.Row(c).Justify(ui.End).Children(func() {
					ActionButton(c.Key("save"), icons, locales.SaveSettings(), "check", false).OnClick(func() {
						config := *draft
						config.Options = maps.Clone(draft.Options)
						for id, value := range config.Options {
							value.APIKey = strings.TrimSpace(value.APIKey)
							value.BaseURL = strings.TrimRight(strings.TrimSpace(value.BaseURL), "/")
							value.SearchEngineID = strings.TrimSpace(value.SearchEngineID)
							if value == (search.Options{}) {
								delete(config.Options, id)
							} else {
								config.Options[id] = value
							}
						}
						if err := save(config); err != nil {
							form.err = err.Error()
							return
						}
						*form = WebSearchForm{}
					})
				})
			}
		})
	})
}

func searchProviderLabel(id search.ID) string {
	switch id {
	case search.DuckDuckGo:
		return "DuckDuckGo"
	case search.SearXNG:
		return "SearXNG"
	case search.Google:
		return locales.SearchProviderGoogle()
	case search.Wikipedia:
		return locales.SearchProviderWikipedia()
	case search.Tavily:
		return "Tavily"
	case search.Firecrawl:
		return "Firecrawl"
	case search.DeepSeek:
		return "DeepSeek"
	case search.Baidu:
		return locales.SearchProviderBaidu()
	case search.Aliyun:
		return locales.SearchProviderAliyun()
	case search.Tencent:
		return locales.SearchProviderTencent()
	case search.Volcengine:
		return locales.SearchProviderVolcengine()
	}
	return ""
}
