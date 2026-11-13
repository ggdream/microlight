package settings

import (
	"strings"

	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/skills"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
)

func SkillList(c *ui.Context, icons asseticon.Set, catalog *skills.Catalog, roots []string, query, expanded *string, refresh func()) {
	loading := catalog == nil
	Section(c, locales.SettingsSkills(), locales.SkillsSettingsHint(), func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			label := locales.SearchSkills()
			theme.TextInput(c.Key("skill-search"), query).Label(label).Placeholder(label).
				Grow(1).MinWidth(0).Height(35).PaddingX(10).FontSize(13).
				Background(theme.White).Border(1, theme.Border).Radius(7).FocusRingColor(theme.Accent)
			ActionButton(c.Key("rescan"), icons, locales.RefreshSkills(), "retry", loading).OnClick(refresh)
		})
		if loading {
			ui.Text(c, locales.ScanningSkills()).FontSize(12).TextColor(theme.Muted)
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
			ui.Text(c, label).FontSize(13).TextColor(theme.Muted)
		} else {
			ui.Column(c.Key("skill-list")).FillWidth().Background(theme.White).Radius(11).Border(1, theme.Border).Clip().Children(func() {
				for i, entry := range matches {
					if i != 0 {
						ui.Box(c).FillWidth().Height(1).Background(theme.Border)
					}
					ui.Column(c.Key(entry.Name)).FillWidth().Children(func() {
						SkillRow(c, icons, entry, expanded)
					})
				}
			})
		}
		for _, path := range roots {
			ui.Text(c, path).FontSize(11).TextColor(theme.Muted).Selectable()
		}
		if len(catalog.Issues) != 0 {
			Card(c, locales.SkillScanIssues(), func() {
				for _, issue := range catalog.Issues {
					ui.Text(c, issue.Error()).FontSize(12).TextColor(theme.Muted).Selectable()
				}
			})
		}
	})
}

func SkillRow(c *ui.Context, icons asseticon.Set, entry skills.Entry, selected *string) {
	expanded := *selected == entry.Name
	button := theme.PointerButton(c.Key("details"), false).Label(entry.Name).Expanded(expanded).FillWidth().Height(56).
		PaddingX(13).Gap(11).Justify(ui.Start).AlignItems(ui.Center)
	if expanded || button.Hovered() {
		button.Background(theme.Side)
	}
	button.Children(func() {
		ui.Box(c).Size(30, 30).Radius(7).Shrink(0).Background(theme.Tint).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() { ui.Icon(c, icons.SVG("document_text")).Size(16, 16).Shrink(0).TextColor(theme.Accent) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, entry.Name).FontSize(13).FontWeight(630).SingleLine().Ellipsis("…")
			if !expanded {
				ui.Text(c, strings.Join(strings.Fields(entry.Description), " ")).FontSize(12).
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
		ui.Column(c.Key("description")).FillWidth().Gap(8).Padding(0, 14, 14, 54).Background(theme.Side).Children(func() {
			ui.Text(c, entry.Description).FontSize(13).TextColor(theme.Ink).Selectable()
			ui.Text(c, entry.Path).FontSize(11).TextColor(theme.Muted).Selectable()
		})
	}
}
