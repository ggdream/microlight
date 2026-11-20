package settings

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/skills"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

func SkillList(c *ui.Context, icons asseticon.Set, catalog *skills.Catalog, roots []string, query, expanded *string, refresh func()) {
	loading := catalog == nil
	scroll := Section(c, locales.SettingsSkills(), locales.SkillsSettingsHint(), func() {
		ui.Row(c).FillWidth().Gap(settingGap(c, 10)).AlignItems(ui.Center).Children(func() {
			label := locales.SearchSkills()
			TextField(c.Key("skill-search"), label, label, query).
				Grow(1).MinWidth(0).ReturnKey(ui.ReturnSearch)
			ActionButton(c.Key("rescan"), icons, locales.RefreshSkills(), "retry", loading).OnClick(refresh)
		})
		if loading {
			ui.Text(c, locales.ScanningSkills()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
			return
		}
		filter := strings.ToLower(strings.TrimSpace(*query))
		var matches []skills.Entry
		for _, entry := range catalog.Entries {
			if strings.Contains(strings.ToLower(entry.Name), filter) || strings.Contains(strings.ToLower(entry.Description), filter) {
				matches = append(matches, entry)
			}
		}
		if len(matches) == 0 {
			label := locales.NoSkillsFound()
			if filter != "" {
				label = locales.NoMatchingSkills()
			}
			ui.Text(c, label).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
		} else {
			material3.CardStyle(ui.Column(c.Key("skill-list")).FillWidth(), material3.ElevatedCard).Clip().Children(func() {
				for i, entry := range matches {
					if i != 0 {
						material3.Divider(c, material3.DividerOptions{Space: 1})
					}
					ui.Column(c.Key(entry.Name)).FillWidth().Children(func() {
						SkillRow(c, icons, entry, expanded)
					})
				}
			})
		}
		paths := func() {
			for _, path := range roots {
				ui.Text(c, path).FillWidth().FontSize(viewport.Font(c, 11)).TextColor(theme.Muted).Selectable()
			}
		}
		if settingsLayout(c).Compact() && len(roots) > 0 {
			state := ui.Box(c.Key("directory-state"))
			open := ui.Local(state, "expanded", func() bool { return false })
			ui.Collapsible(c.Key("directories"), locales.SkillDirectories(), open, paths)
		} else {
			paths()
		}
		if len(catalog.Issues) != 0 {
			Card(c, locales.SkillScanIssues(), func() {
				for _, issue := range catalog.Issues {
					ui.Text(c, issue.Error()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted).Selectable()
				}
			})
		}
	})
	refreshState := ui.Local(scroll, "pull-to-refresh", func() ui.RefreshState { return ui.RefreshState{} })
	refreshState.Refreshing = loading
	if scroll.PullToRefresh(refreshState) && !loading {
		refresh()
	}
}

func SkillRow(c *ui.Context, icons asseticon.Set, entry skills.Entry, selected *string) {
	expanded := *selected == entry.Name
	button := theme.PointerButton(c.Key("details"), false).Label(entry.Name).Expanded(expanded).FillWidth().MinHeight(settingsLayout(c).Target(56)).
		PaddingX(13).Gap(settingGap(c, 11)).Justify(ui.Start).AlignItems(ui.Center)
	if expanded || button.Hovered() {
		button.Background(theme.Hover)
	}
	button.Children(func() {
		ui.Box(c).Size(30, 30).Radius(7).Shrink(0).Background(theme.Tint).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() { ui.Icon(c, icons.SVG("document_text")).Size(16, 16).Shrink(0).TextColor(theme.Accent) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(settingGap(c, 3)).Children(func() {
			ui.Text(c, entry.Name).FontSize(viewport.Font(c, 13)).FontWeight(630).SingleLine().Ellipsis("…")
			if !expanded {
				ui.Text(c, strings.Join(strings.Fields(entry.Description), " ")).FontSize(viewport.Font(c, 12)).
					TextColor(theme.Muted).SingleLine().Ellipsis("…")
			}
		})
		chevron := ui.Icon(c, icons.SVG("ios_chevron_right")).Size(13, 13).Shrink(0).TextColor(theme.Muted)
		if expanded {
			chevron.Rotate(90)
		}
	}).OnClick(func() {
		if *selected == entry.Name {
			*selected = ""
		} else {
			*selected = entry.Name
		}
	})
	if expanded {
		ui.Column(c.Key("description")).FillWidth().Gap(settingGap(c, 8)).Padding(0, settingX(c, 14), settingY(c, 14), settingX(c, 54)).Background(theme.Side).Children(func() {
			ui.Text(c, entry.Description).FontSize(viewport.Font(c, 13)).TextColor(theme.Ink).Selectable()
			ui.Text(c, entry.Path).FontSize(viewport.Font(c, 11)).TextColor(theme.Muted).Selectable()
		})
	}
}
