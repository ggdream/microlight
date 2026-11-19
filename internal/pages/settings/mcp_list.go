package settings

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"net/url"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/theme"
)

func (form *MCPForm) serverList(c *ui.Context, icons asseticon.Set, servers []mcptools.Server, save func([]mcptools.Server) error, navigate func(func())) {
	ui.Column(c).FillWidth().Background(theme.White).Radius(13).Border(1, theme.Border).
		Padding(settingsLayout(c).Density(12, 8)).Gap(settingGap(c, 8)).Children(func() {
		ui.Row(c.Key("header")).FillWidth().AlignItems(ui.Center).Gap(settingGap(c, 10)).PaddingX(settingX(c, 4)).Children(func() {
			ui.Text(c, locales.McpServers()).Grow(1).MinWidth(0).FontSize(viewport.Font(c, 15)).FontWeight(660)
			ActionButton(c.Key("add"), icons, locales.AddMCPServer(), "plus", false).OnClick(func() {
				if form.adding {
					form.revealEditor = true
					return
				}
				navigate(func() {
					form.reset()
					form.adding, form.revealEditor = true, true
				})
			})
		})
		if len(servers) == 0 {
			ui.Text(c, locales.McpNoServers()).Padding(settingY(c, 8), settingX(c, 4)).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
		}
		for i, server := range servers {
			edit := func() {
				if form.mcpEditing == server.Name {
					form.revealEditor = true
					return
				}
				navigate(func() {
					form.edit(server)
					form.revealEditor = true
				})
			}
			remove := func() {
				discard := func() {
					if form.save(save, slices.Delete(slices.Clone(servers), i, i+1)) && form.mcpEditing == server.Name {
						form.reset()
					}
				}
				if form.mcpEditing == server.Name {
					navigate(discard)
				} else {
					discard()
				}
			}
			mcpServerRow(c.Key(server.Name), icons, server, form.mcpEditing == server.Name, edit, remove)
		}
	})
}

func mcpServerRow(c *ui.Context, icons asseticon.Set, server mcptools.Server, selected bool, edit, remove func()) {
	layout := settingsLayout(c)
	transport := server.Kind()
	target, detail, glyph := server.URL, server.URL, "globe"
	if transport == mcptools.Stdio {
		target, detail, glyph = server.Command, filepath.Base(server.Command), "window_console"
	} else if endpoint, err := url.Parse(server.URL); err == nil && endpoint.Host != "" {
		detail = endpoint.Host
	}
	chipBackground := theme.Tint
	row := ui.Row(c).FillWidth().MinWidth(0).MinHeight(layout.Target(50)).Padding(settingY(c, 3), settingX(c, 6), settingY(c, 3), settingX(c, 3)).
		AlignItems(ui.Center).Gap(settingGap(c, 5)).Radius(9).Background(theme.Canvas)
	if selected {
		row.Background(theme.Tint)
		chipBackground = theme.White
	}
	row.Children(func() {
		button := theme.PointerButton(c.Key("edit"), false).Label(server.Name).Tooltip(target).
			Grow(1).MinWidth(0).Padding(settingY(c, 5), settingX(c, 7)).Gap(settingsLayout(c).Density(10, 6)).AlignItems(ui.Center).
			Radius(7).FocusRingColor(theme.Accent)
		if button.Hovered() && !selected {
			button.Background(theme.Side)
		}
		button.Children(func() {
			ui.Box(c).Size(31, 31).Shrink(0).Center().Background(theme.White).
				Border(1, theme.Border).Radius(8).Children(func() {
				ui.Icon(c, icons.SVG(glyph)).Size(16, 16).TextColor(theme.Accent)
			})
			ui.Column(c).Grow(1).MinWidth(0).Clip().Gap(settingGap(c, 2)).Children(func() {
				ui.Text(c, server.Name).FillWidth().MinWidth(0).SingleLine().Ellipsis("…").
					FontSize(viewport.Font(c, 13)).FontWeight(620)
				metadata := ui.Row
				align := ui.Center
				if layout.Compact() {
					metadata = ui.Column
					align = ui.Start
				}
				metadata(c).AlignItems(align).FillWidth().MinWidth(0).Gap(settingGap(c, layout.Density(6, 3))).Children(func() {
					ui.Box(c).MaxWidthPercent(100).Shrink(0).Padding(settingY(c, 2), settingX(c, 6)).Radius(999).Background(chipBackground).Children(func() {
						ui.Text(c, transport.Label()).SingleLine().FontSize(viewport.Font(c, 10)).FontWeight(600).TextColor(theme.Accent)
					})
					ui.Text(c, detail).Grow(1).MinWidth(0).MaxWidthPercent(100).SingleLine().Ellipsis("…").
						FontSize(viewport.Font(c, 11)).TextColor(theme.Muted)
				})
			})
		}).OnClick(edit)

		ui.Themed(c.Key("actions"), theme.PopupTheme(c)).Background(ui.Transparent).Shrink(0).Children(func() {
			label := locales.McpServerActions()
			size := layout.Target(31)
			trigger := theme.PointerButton(c, false).Label(label).Tooltip(label).Size(size, size).
				Shrink(0).Center().Radius(7).FocusRingColor(theme.Accent)
			if trigger.Hovered() {
				trigger.Background(theme.Hover)
			}
			trigger.Children(func() {
				ui.Icon(c, icons.SVG("more_horizontal")).Size(17, 17).TextColor(theme.Muted)
			}).PopoverMenuButton(func(menu *ui.Menu) {
				if menu.Item(locales.EditMcpServer()).Icon(icons.Menu("edit")).Chosen() {
					edit()
				}
				if menu.Item(locales.RemoveMCPServer()).Icon(icons.Menu("delete")).Danger().Chosen() {
					remove()
				}
			}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd})
		})
	})
}
