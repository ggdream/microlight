package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/viewport"
)

func settingsLayout(c *ui.Context) viewport.Layout {
	layout := viewport.Available(c)
	if layout.Width >= viewport.CompactWidth {
		layout.Width -= settingsSidebarWidth
	}
	return layout
}

// SelectHeight keeps form selectors compact without scaling down their text.
func SelectHeight(c *ui.Context) float32 {
	layout := settingsLayout(c)
	if layout.Compact() {
		return max(34, layout.Y(36))
	}
	return layout.Target(35)
}

func Section(c *ui.Context, title, subtitle string, content func()) ui.Element {
	return section(c, func() {
		ui.Column(c).Gap(settingGap(c, 5)).Children(func() {
			ui.Text(c, title).FontSize(viewport.Font(c, 29)).FontWeight(680)
			ui.Text(c, subtitle).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
		})
	}, content)
}

func section(c *ui.Context, header, content func()) ui.Element {
	layout := settingsLayout(c)
	scroll, offset := elastic.Scroll(c)
	return scroll.Grow(1).MinHeight(0).Children(func() {
		offset.Apply(ui.Column(c)).FillWidth().AlignItems(ui.Center).Padding(layout.Y(layout.Density(26, 14)), layout.Gutter(22)).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(610).Gap(layout.Y(layout.Density(16, 7))).Children(func() {
				header()
				content()
			})
		})
	})
}

func Footer(c *ui.Context, message string, failed bool, save func()) {
	layout := settingsLayout(c)
	ui.Column(c).Shrink(0).AlignItems(ui.Center).Padding(layout.Y(11), layout.Gutter(22)).
		Background(theme.Canvas).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().MaxWidth(610).AlignItems(ui.Center).Gap(settingGap(c, 12)).Children(func() {
			color := theme.Accent
			if failed {
				color = theme.ErrorFG
			}
			if message != "" && (layout.Compact() || layout.Height < 420) {
				scroll, offset := elastic.Scroll(c.Key("save-status"))
				scroll.Grow(1).MinWidth(0).MaxHeight(layout.Target(48)).Children(func() {
					offset.Apply(ui.Text(c, message)).FillWidth().FontSize(viewport.Font(c, 12)).TextColor(color).Selectable()
				})
			} else {
				ui.Text(c, message).Grow(1).MinWidth(0).FontSize(viewport.Font(c, 12)).TextColor(color)
			}
			button := theme.PointerButton(c, false).Label(locales.SaveSettings()).Height(layout.Target(36)).Shrink(0).PaddingX(settingX(c, 15)).
				Radius(8).Background(theme.Accent).AlignItems(ui.Center)
			if button.Hovered() {
				button.Background(ui.Hex("#115541"))
			}
			button.Children(func() {
				ui.Text(c, locales.SaveSettings()).FontSize(viewport.Font(c, 13)).FontWeight(620).TextColor(theme.White)
			}).OnClick(save)
		})
	})
}

func Card(c *ui.Context, title string, fields func()) ui.Element {
	layout := settingsLayout(c)
	return ui.Column(c).FillWidth().Background(theme.White).Radius(13).Border(1, theme.Border).
		Padding(layout.Y(layout.Density(15, 7)), layout.X(layout.Density(16, 11))).Gap(layout.Y(layout.Density(11, 6))).Children(func() {
		ui.Text(c, title).FontSize(viewport.Font(c, 15)).FontWeight(660)
		fields()
	})
}

func Field(c *ui.Context, label, placeholder string, value *string) ui.Element {
	var input ui.Element
	ui.Column(c).Grow(1).MinWidth(0).Gap(settingGap(c, 5)).Children(func() {
		ui.Text(c, label).FontSize(viewport.Font(c, 12)).FontWeight(610)
		input = TextField(c.Key(label), label, placeholder, value).FillWidth()
	})
	return input
}

func TextField(c *ui.Context, label, placeholder string, value *string) ui.Element {
	return theme.InputFrame(theme.TextInput(c, value), 10, 7, theme.Border).
		Label(label).Placeholder(placeholder).Height(settingsLayout(c).Target(35)).FontSize(viewport.Font(c, 13)).
		Background(theme.Canvas).FocusRingColor(theme.Accent)
}

func AboutRow(c *ui.Context, row ui.Element) ui.Element {
	return row.FillWidth().MinHeight(44).AlignItems(ui.Center).Gap(settingGap(c, 12)).
		Background(theme.White).Radius(13).Border(1, theme.Border).PaddingX(settingX(c, 16))
}

func AboutPanel(c *ui.Context, icons asseticon.Set, version func(*ui.Context)) {
	section(c, func() {
		ui.Text(c, locales.AppAbout()).FontSize(viewport.Font(c, 29)).FontWeight(680)
	}, func() {
		version(c.Key("version"))
		repository(c.Key("repository"), icons)
	})
}

func repository(c *ui.Context, icons asseticon.Set) {
	const repositoryPath = "ggdream/microlight"
	label := locales.RepositoryAddress()
	button := AboutRow(c, theme.PointerButton(c, false).Label(label)).FocusRingColor(theme.Accent)
	if button.Hovered() {
		button.Background(theme.Hover)
	}
	button.Children(func() {
		ui.Text(c, label).Grow(1).MinWidth(0).FontSize(viewport.Font(c, 15)).FontWeight(660)
		ui.Row(c).AlignItems(ui.Center).Gap(settingGap(c, 7)).Shrink(0).Children(func() {
			ui.Icon(c, icons.SVG("github")).Size(16, 16).TextColor(theme.Ink)
			ui.Text(c, repositoryPath).FontSize(viewport.Font(c, 13)).TextColor(theme.Ink)
			ui.Icon(c, icons.SVG("open")).Size(14, 14).TextColor(theme.Muted)
		})
	}).OnClick(func() {
		c.OpenURL("https://github.com/" + repositoryPath)
	})
}

// Settings keep their spacing in proportion to the pane available beside
// navigation. Compact forms use a tighter base before applying each axis.
func settingX(c *ui.Context, points float32) float32 {
	layout := settingsLayout(c)
	return max(points*.76, layout.X(layout.Density(points, points*.9)))
}

func settingY(c *ui.Context, points float32) float32 {
	layout := settingsLayout(c)
	return max(points*.76, layout.Y(layout.Density(points, points*.9)))
}

func settingGap(c *ui.Context, points float32) float32 {
	return min(settingX(c, points), settingY(c, points))
}
