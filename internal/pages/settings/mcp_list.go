package settings

import (
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
		Padding(12, 12).Gap(8).Children(func() {
		ui.Row(c.Key("header")).FillWidth().AlignItems(ui.Center).Gap(10).PaddingX(4).Children(func() {
			ui.Text(c, locales.McpServers()).Grow(1).MinWidth(0).FontSize(15).FontWeight(660)
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
			ui.Text(c, locales.McpNoServers()).Padding(8, 4).FontSize(13).TextColor(theme.Muted)
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
	transport := server.Kind()
	target, detail, glyph := server.URL, server.URL, "globe"
	if transport == mcptools.Stdio {
		target, detail, glyph = server.Command, filepath.Base(server.Command), "window_console"
	} else if endpoint, err := url.Parse(server.URL); err == nil && endpoint.Host != "" {
		detail = endpoint.Host
	}
	chipBackground := theme.Tint
	row := ui.Row(c).FillWidth().MinWidth(0).MinHeight(50).Padding(3, 6, 3, 3).
		AlignItems(ui.Center).Gap(5).Radius(9).Background(theme.Canvas)
	if selected {
		row.Background(theme.Tint)
		chipBackground = theme.White
	}
	row.Children(func() {
		button := theme.PointerButton(c.Key("edit"), false).Label(server.Name).Tooltip(target).
			Grow(1).MinWidth(0).Padding(5, 7).Gap(10).AlignItems(ui.Center).
			Radius(7).FocusRingColor(theme.Accent)
		if button.Hovered() && !selected {
			button.Background(theme.Side)
		}
		button.Children(func() {
			ui.Box(c).Size(31, 31).Shrink(0).Center().Background(theme.White).
				Border(1, theme.Border).Radius(8).Children(func() {
				ui.Icon(c, icons.SVG(glyph)).Size(16, 16).TextColor(theme.Accent)
			})
			ui.Column(c).Grow(1).MinWidth(0).Clip().Gap(2).Children(func() {
				ui.Text(c, server.Name).FillWidth().MinWidth(0).SingleLine().Ellipsis("…").
					FontSize(13).FontWeight(620)
				ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Box(c).Shrink(0).Padding(2, 6).Radius(999).Background(chipBackground).Children(func() {
						ui.Text(c, transport.Label()).SingleLine().FontSize(10).FontWeight(600).TextColor(theme.Accent)
					})
					ui.Text(c, detail).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
						FontSize(11).TextColor(theme.Muted)
				})
			})
		}).OnClick(edit)

		ui.Themed(c.Key("actions"), theme.MenuTheme).Background(ui.Transparent).Shrink(0).Children(func() {
			label := locales.McpServerActions()
			trigger := theme.PointerButton(c, false).Label(label).Tooltip(label).Size(31, 31).
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
