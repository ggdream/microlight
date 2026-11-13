package settings

import (
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
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
	ui.Row(c.Key("interface-language")).FillWidth().AlignItems(ui.Center).Gap(12).
		Background(theme.White).Radius(13).Border(1, theme.Border).Padding(12, 16).Children(func() {
		ui.Text(c, title).Grow(1).MinWidth(0).FontSize(15).FontWeight(660)
		selected := current
		dropdown := ui.SelectBase(c.Key("language-select"), &selected)
		trigger := dropdown.Trigger.Label(title).Value(labels[selected]).Width(148).Height(34).Shrink(0).PaddingX(10).
			AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Border(1, theme.Border).Radius(7).
			Background(theme.Canvas).Cursor(ui.CursorPointer).FocusRingColor(theme.Accent)
		if trigger.Hovered() {
			trigger.Background(theme.Side)
		}
		trigger.Children(func() {
			ui.Text(c, labels[selected]).FontSize(12).TextColor(theme.Ink)
			ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Shrink(0).Rotate(90).TextColor(theme.Muted)
		}).OnChange(func() { choose(selected) })
		dropdown.Popup(func(panel ui.Element) {
			panel.Padding(4).Gap(2).Background(theme.White).Border(1, theme.Border).Radius(8)
			for _, option := range []LanguagePreference{SystemLanguage, Chinese, English} {
				item := dropdown.Item(option).Label(labels[option]).Padding(6, 8).Radius(5).
					AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Cursor(ui.CursorPointer)
				if item.Highlighted() {
					item.Background(theme.Tint)
				}
				item.Children(func() {
					ui.Text(c, labels[option]).FontSize(12).TextColor(theme.Ink)
					if option == selected {
						ui.Icon(c, icons.SVG("check")).Size(14, 14).Shrink(0).TextColor(theme.Accent)
					}
				})
			}
		})
	})
}
