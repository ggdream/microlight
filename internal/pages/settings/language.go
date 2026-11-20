package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

type LanguagePreference string

const (
	SystemLanguage LanguagePreference = "system"
	English        LanguagePreference = "en"
	Chinese        LanguagePreference = "zh-Hans"
)

func Language(c *ui.Context, icons asseticon.Set, current LanguagePreference, choose func(LanguagePreference)) {
	labels := map[LanguagePreference]string{
		SystemLanguage: locales.FollowSystem(), Chinese: "简体中文", English: "English",
	}
	title := locales.InterfaceLanguage()
	layout := settingsLayout(c)
	material3.CardStyle(ui.Row(c.Key("interface-language")).FillWidth().AlignItems(ui.Center).Gap(layout.Density(12, 8)),
		material3.ElevatedCard).Padding(layout.Y(layout.Density(12, 5)), layout.X(layout.Density(16, 11))).Children(func() {
		ui.Text(c, title).Grow(1).MinWidth(0).FontSize(viewport.Font(c, 15)).FontWeight(660)
		selected := current
		dropdown := ui.SelectBase(c.Key("language-select"), &selected)
		trigger := material3.SelectFrame(dropdown.Trigger.Label(title).Value(labels[selected]).Width(layout.Density(148, 120)).MaxWidthPercent(50).Height(SelectHeight(c)).Shrink(0).PaddingX(settingX(c, 10)).
			AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(settingGap(c, 8))).Radius(material3.ShapeSmall).
			Background(material3.Monochrome.SurfaceContainer).Cursor(ui.CursorPointer).FocusRingColor(theme.Accent)
		if trigger.Hovered() {
			trigger.Background(theme.Hover)
		}
		trigger.Children(func() {
			ui.Text(c, labels[selected]).FontSize(viewport.Font(c, 12)).TextColor(theme.Ink)
			ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Shrink(0).Rotate(90).TextColor(theme.Muted)
		}).OnChange(func() { choose(selected) })
		dropdown.Popup(func(panel ui.Element) {
			material3.Menu(panel.Padding(settingY(c, 4), settingX(c, 4)).Gap(settingGap(c, 2))).Radius(material3.ShapeMedium)
			for _, option := range []LanguagePreference{SystemLanguage, Chinese, English} {
				item := dropdown.Item(option).Label(labels[option]).Padding(settingY(c, 6), settingX(c, 8)).Radius(5).
					AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(settingGap(c, 8)).Cursor(ui.CursorPointer)
				if item.Highlighted() {
					item.Background(theme.Hover)
				}
				item.Children(func() {
					ui.Text(c, labels[option]).FontSize(viewport.Font(c, 12)).TextColor(theme.Ink)
					if option == selected {
						ui.Icon(c, icons.SVG("check")).Size(14, 14).Shrink(0).TextColor(theme.Accent)
					}
				})
			}
		})
	})
}
