package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/viewport"
)

type Tab int

const (
	settingsNavInset     = 10
	settingsSidebarWidth = 190
)

const (
	General Tab = iota
	Model
	Skills
	Tools
	Archive
	About
)

func Page(c *ui.Context, icons asseticon.Set, selected Tab, back func(), selectTab func(Tab), render func(*ui.Context, Tab)) {
	layout := viewport.Available(c)
	items := []struct {
		tab          Tab
		label, glyph string
	}{
		{General, locales.SettingsGeneral(), "settings"},
		{Model, locales.SettingsModel(), "brain_circuit"},
		{Skills, locales.SettingsSkills(), "skill"},
		{Tools, locales.SettingsTools(), "plug_connected"},
		{Archive, locales.ArchivedChats(), "archive"},
		{About, locales.AppAbout(), "info"},
	}
	body := func() {
		column := ui.Column(c.Key("content")).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch)
		if !layout.Compact() {
			column.FillHeight()
		}
		column.Children(func() { render(c, selected) })
	}
	if layout.Compact() {
		top, _, _, _ := c.SafeArea()
		if top > 0 {
			ui.Overlay(c, func() {
				ui.Box(c.Key("status-bar-background")).Absolute().Top(0).Left(0).Right(0).
					Height(top).Background(theme.Side).IgnoreInput(true)
			})
		}
		ui.Column(c).Fill().Children(func() {
			ui.Row(c.Key("navigation")).FillWidth().Shrink(0).AlignItems(ui.Center).
				Padding(4, layout.Gutter(12)).Gap(settingGap(c, 8)).Background(theme.Side).Children(func() {
				backButton := theme.PointerButton(c.Key("back"), false).Label(locales.BackToChat()).
					Size(layout.Target(44), layout.Target(44)).Center().Shrink(0)
				backButton.Children(func() {
					ui.Icon(c, icons.SVG("back")).Size(18, 18).TextColor(theme.Ink)
				}).OnClick(back)
				ui.Box(c).Grow(1)
				popupItems := make([]theme.PopupItem, 0, len(items))
				for _, entry := range items {
					popupItems = append(popupItems, theme.PopupItem{
						Label: entry.label, Icon: icons.Menu(entry.glyph), Selected: selected == entry.tab,
						OnSelect: func() { selectTab(entry.tab) },
					})
				}
				theme.PopupMenuAt(c.Key("section-menu"), func(c *ui.Context) ui.Element {
					size := layout.Target(44)
					button := theme.PointerButton(c, false).Label(locales.Settings()).Size(size, size).
						Center().Radius(8).Shrink(0)
					if button.Hovered() {
						button.Background(theme.Hover)
					}
					return button.Children(func() {
						ui.Icon(c, icons.SVG("navigation")).Size(20, 20).TextColor(theme.Ink)
					})
				}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd}, popupItems...)
			})
			body()
		})
		return
	}
	ui.Row(c).Fill().Children(func() {
		ui.Column(c).Width(settingsSidebarWidth).FillHeight().Shrink(0).Background(theme.Side).
			BorderWidth(0, 1, 0, 0).BorderColor(theme.Border).Padding(settingY(c, 15), settingX(c, 12)).Gap(settingGap(c, 6)).Children(func() {
			sidebarButton(c.Key("back"), icons, locales.BackToChat(), "back", false).OnClick(back)
			scroll, offset := elastic.Scroll(c.Key("settings-sections"))
			scroll.FillWidth().Grow(1).MinHeight(0).Children(func() {
				offset.Apply(ui.Column(c)).FillWidth().Gap(settingGap(c, 6)).Children(func() {
					ui.Text(c, locales.Settings()).FontSize(viewport.Font(c, 19)).FontWeight(660).Padding(settingY(c, 14), settingX(c, settingsNavInset), settingY(c, 7))
					for _, entry := range items {
						sidebarButton(c.Key(entry.tab), icons, entry.label, entry.glyph, selected == entry.tab).
							OnClick(func() { selectTab(entry.tab) })
					}
				})
			})
		})
		body()
	})
}

func sidebarButton(c *ui.Context, icons asseticon.Set, label, glyph string, selected bool) ui.Element {
	button := theme.PointerButton(c, false).Label(label).FillWidth().Height(settingsLayout(c).Target(36)).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).PaddingX(settingX(c, settingsNavInset)).Gap(settingGap(c, 9))
	color := theme.Muted
	if selected {
		button.Background(theme.Tint)
		color = theme.Accent
	} else if button.Hovered() {
		button.Background(theme.Hover)
	}
	return button.Children(func() {
		ui.Icon(c, icons.SVG(glyph)).Size(16, 16).Shrink(0).TextColor(color)
		ui.Text(c, label).FontSize(viewport.Font(c, 13)).FontWeight(590).TextColor(color)
	})
}
