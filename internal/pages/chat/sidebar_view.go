package chat

import (
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/viewport"
	"time"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
)

func (app *chatApp) sidebarDivider(c *ui.Context, limit float32) float32 {
	handle := ui.Box(c.Key("sidebar-divider")).Absolute().Top(0).Bottom(0).Width(7).ZIndex(1).
		Focusable().FocusRing(false).Cursor(ui.CursorResizeEW).Role(ui.RoleSplitter).Label(locales.ResizeSidebar())
	drag := ui.Local(handle, "offset", func() float32 { return 0 })
	dx, _, held := handle.Dragged()
	snap := func(width float32) float32 {
		if width <= sidebarCollapseAt {
			return minSidebarWidth
		}
		return min(width, limit)
	}
	switch {
	case handle.DoubleClicked():
		app.sidebarWidth = defaultSidebarWidth
		*drag = 0
	case held:
		// Keep tracking the pointer through the widths hidden by snapping.
		*drag = min(max(app.sidebarWidth+*drag+dx, minSidebarWidth), limit) - app.sidebarWidth
	case handle.PressCanceled():
		*drag = 0
	case *drag != 0:
		app.sidebarWidth = snap(app.sidebarWidth + *drag)
		*drag = 0
	}
	if handle.Shortcut(0, ui.KeyLeft) {
		app.sidebarWidth = snap(app.sidebarWidth - 10)
	}
	if handle.Shortcut(0, ui.KeyRight) {
		app.sidebarWidth = snap(max(app.sidebarWidth, sidebarCollapseAt) + 10)
	}
	shown := snap(app.sidebarWidth + *drag)
	handle.Left(shown - 3).Draw(func(p *ui.Painter, r ui.Rect) {
		color := border
		if held {
			color = accent
		} else if handle.Hovered() || handle.FocusVisible() {
			color = accent.Alpha(.55)
		}
		p.Fill(ui.Rect{X: r.X + 3, Y: r.Y, W: 1, H: r.H}, color, 0)
	})
	return shown
}

func (app *chatApp) historyDrawer(c *ui.Context, windowWidth float32, selected *conversation) {
	const drawerMaxWidth, scrimMinWidth = 304, 56
	width := min(float32(drawerMaxWidth), windowWidth-scrimMinWidth)
	closeDrawer := func() { app.compactHistory = false }
	top, _, bottom, left := c.SafeArea()
	ui.Overlay(c, func() {
		layer := ui.Row(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
			Background(ui.RGBA(0, 0, 0, .32)).Modal()
		if layer.OverlayShortcut(0, ui.KeyEscape) {
			closeDrawer()
		}
		layer.Children(func() {
			ui.Box(c.Key("panel")).Width(width+left).FillHeight().Shrink(0).Background(side).Clip().
				Padding(top, 0, bottom, left).Children(func() { app.sidebar(c.Key("history"), selected, "", width) })
			pointerButton(c.Key("scrim"), false).Label(locales.CollapseSidebar()).Grow(1).FillHeight().
				OnClick(closeDrawer)
		})
	})
}

func (app *chatApp) sidebar(c *ui.Context, selected *conversation, shortcut string, width float32) {
	if width == minSidebarWidth {
		ui.Column(c).Fill().AlignItems(ui.Center).Padding(20, 0, 18).Background(side).Children(func() {
			app.sidebarToggle(c, true)
			ui.Box(c).Grow(1)
			app.sidebarSettingsButton(c)
		})
		return
	}
	const sidebarInset = 14
	layout := windowLayout(c)
	background := side
	if layout.Compact() {
		background = ui.Transparent
	}
	bottomInset := layout.Y(layout.Density(18, 8))
	ui.Themed(c, theme.PopupTheme(c)).Fill().Background(ui.Transparent).Children(func() {
		ui.Column(c).Fill().Padding(layout.Y(20), 0, bottomInset, sidebarInset).Background(background).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(0, 0, 0, 4).Margin(0, sidebarInset, 24, 0).Children(func() {
				ui.Image(c, app.brandIcon).Size(36, 36).Shrink(0).Fit(ui.Contain)
				ui.Text(c, locales.AppName()).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 16)).FontWeight(660)
				if layout.Compact() {
					app.compactNavButton(c.Key("close-history"), locales.BackToChat(), compactHistoryIcon, func() { app.compactHistory = false })
				} else {
					app.sidebarToggle(c, false)
				}
			})
			button := material3.ButtonStyle(pointerButton(c, false).Label(locales.NewChat()).AlignSelf(ui.Stretch).Height(windowLayout(c).Target(38)).PaddingX(10).
				Margin(0, sidebarInset, 0, 0), material3.ElevatedButton, false).AlignItems(ui.Center).Gap(7)
			if button.Hovered() {
				button.Background(hover)
			}
			button.Children(func() {
				icon(c, "plus", accent, 15)
				ui.Text(c, locales.NewChat()).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13)).FontWeight(610)
				if width >= 180 && shortcut != "" {
					ui.Text(c, shortcut).FontSize(viewport.Font(c, 10)).TextColor(muted)
				}
			}).OnClick(func() { app.selectChat("") })

			ui.Text(c, locales.ChatHistory()).FontSize(viewport.Font(c, 12)).FontWeight(620).TextColor(muted).PaddingX(7).Margin(windowLayout(c).Y(24), 0, windowLayout(c).Y(8))
			listTheme := *c.Theme()
			listTheme.ScrollbarWidth = 4
			listTheme.Scrollbar = muted.Alpha(0.36)
			// The scroll area reaches the sidebar edge, but its rows share the button's inset.
			ui.Themed(c, &listTheme).Grow(1).MinHeight(0).Background(ui.Transparent).Children(func() {
				scroll, offset := elastic.Scroll(c)
				scroll.Fill().MinHeight(0).Children(func() {
					offset.Apply(ui.Column(c)).FillWidth().Padding(0, sidebarInset, 0, 0).Gap(layout.Density(3, 2)).Children(func() {
						chats := app.sidebarChats()
						if len(chats) == 0 {
							ui.Text(c, locales.NoChats()).FontSize(viewport.Font(c, 11)).FontWeight(400).Padding(7).TextColor(muted.Alpha(.8))
						}
						for i, chat := range chats {
							if i > 0 && chats[i-1].pinned && !chat.pinned {
								ui.Box(c.Key("pinned-divider")).Height(1).Margin(4, 8).Background(border)
							}
							app.sidebarChat(c.Key(chat.ID), chat, selected == chat)
						}
					})
				})
			})
			material3.EdgeDivider(ui.Row(c).AlignSelf(ui.Stretch).Margin(0, sidebarInset, 0, 0).Padding(layout.Density(8, layout.Y(8)), 0, 0).Gap(8).AlignItems(ui.Center), material3.TopEdge).
				Children(func() {
					app.sidebarSettingsButton(c)
					ui.Text(c, locales.HistoryLocal()).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, layout.Density(11, 12))).TextColor(muted)
				})
		})
	})
}

func (app *chatApp) sidebarToggle(c *ui.Context, collapsed bool) {
	label, glyph, width := locales.CollapseSidebar(), "panel_left_contract", float32(minSidebarWidth)
	if collapsed {
		label, glyph, width = locales.ExpandSidebar(), "panel_left_expand", defaultSidebarWidth
	}
	button := pointerButton(c.Key("sidebar-toggle"), false).Label(label).Tooltip(label).
		Size(32, 32).Shrink(0).Radius(7).AlignItems(ui.Center).Justify(ui.Center)
	if button.Hovered() {
		button.Background(hover)
	}
	button.Children(func() {
		icon(c, glyph, muted, 18)
	}).OnClick(func() { app.sidebarWidth = width })
}

func (app *chatApp) sidebarSettingsButton(c *ui.Context) {
	label := locales.Settings()
	button := pointerButton(c.Key("sidebar-settings"), false).Label(label).Tooltip(label).Size(32, 32).Shrink(0).
		Radius(7).AlignItems(ui.Center).Justify(ui.Center)
	color := muted
	if app.settingsOpen {
		button.Background(tint)
		color = accent
	} else if button.Hovered() {
		button.Background(hover)
	}
	button.Children(func() {
		icon(c, "settings", color, windowLayout(c).Density(16, 18))
	}).OnClick(app.openSettings)
}

func drawActivityRing(p *ui.Painter, r ui.Rect) {
	const turn = 1600 * time.Millisecond
	angle := float32(p.Now().UnixNano()%int64(turn)) / float32(turn) * 360
	x, y := r.X+r.W/2, r.Y+r.H/2
	var ring ui.Path
	ring.Circle(x, y, 5)
	p.StrokePath(&ring, 1.5, muted.Alpha(.23))
	ring.Trim(0, .74)
	p.Transform(ui.Rotation(angle), x, y, func() {
		p.StrokePath(&ring, 1.5, muted)
	})
	p.AnimationFrame()
}
