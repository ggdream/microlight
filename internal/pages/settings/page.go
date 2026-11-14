package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
)

type Tab int

const settingsNavInset = 10

const (
	General Tab = iota
	Model
	Skills
	Tools
	Archive
	About
)

func Page(c *ui.Context, icons asseticon.Set, selected Tab, back func(), selectTab func(Tab), render func(*ui.Context, Tab)) {
	navItem := func(c *ui.Context, tab Tab, label, glyph string) {
		sidebarButton(c, icons, label, glyph, selected == tab).OnClick(func() { selectTab(tab) })
	}
	ui.Row(c).Fill().Children(func() {
		ui.Column(c).Width(190).FillHeight().Shrink(0).Background(theme.Side).
			BorderWidth(0, 1, 0, 0).BorderColor(theme.Border).Padding(15, 12).Gap(6).Children(func() {
			sidebarButton(c.Key("back"), icons, locales.BackToChat(), "back", false).OnClick(back)
			ui.Text(c, locales.Settings()).FontSize(19).FontWeight(660).Padding(14, settingsNavInset, 7)
			navItem(c.Key("general"), General, locales.SettingsGeneral(), "settings")
			navItem(c.Key("model"), Model, locales.SettingsModel(), "developer_board")
			navItem(c.Key("skills"), Skills, locales.SettingsSkills(), "skill")
			navItem(c.Key("mcp"), Tools, locales.SettingsTools(), "wrench")
			navItem(c.Key("archive"), Archive, locales.ArchivedChats(), "archive")
			navItem(c.Key("about"), About, locales.AppAbout(), "info")
		})
		ui.Column(c).FillHeight().Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			render(c, selected)
		})
	})
}

func sidebarButton(c *ui.Context, icons asseticon.Set, label, glyph string, selected bool) ui.Element {
	button := theme.PointerButton(c, false).Label(label).FillWidth().Height(36).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).PaddingX(settingsNavInset).Gap(9)
	color := theme.Muted
	if selected {
		button.Background(theme.Tint)
		color = theme.Accent
	} else if button.Hovered() {
		button.Background(theme.Hover)
	}
	return button.Children(func() {
		ui.Icon(c, icons.SVG(glyph)).Size(16, 16).Shrink(0).TextColor(color)
		ui.Text(c, label).FontSize(13).FontWeight(590).TextColor(color)
	})
}
