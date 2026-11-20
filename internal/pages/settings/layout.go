package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
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
			material3.Text(c, title, material3.HeadlineMedium)
			material3.Text(c, subtitle, material3.BodyMedium).TextColor(theme.Muted)
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
	material3.EdgeDivider(ui.Column(c).Shrink(0).AlignItems(ui.Center).Padding(layout.Y(11), layout.Gutter(22)).
		Background(theme.Canvas), material3.TopEdge).Children(func() {
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
			button := material3.Button(c, locales.SaveSettings(), material3.FilledButton, false).Height(layout.Target(36)).PaddingX(settingX(c, 18))
			button.Children(func() {
				ui.Text(c, locales.SaveSettings()).FontSize(viewport.Font(c, 13)).FontWeight(620).TextColor(theme.White)
			}).OnClick(save)
		})
	})
}

func Card(c *ui.Context, title string, fields func()) ui.Element {
	return CardWithAction(c, title, nil, fields)
}

func CardWithAction(c *ui.Context, title string, action func(*ui.Context), fields func()) ui.Element {
	layout := settingsLayout(c)
	return material3.Card(c, material3.ElevatedCard).
		Padding(layout.Y(layout.Density(15, 7)), layout.X(layout.Density(16, 11))).Gap(layout.Y(layout.Density(11, 6))).Children(func() {
		if action == nil {
			material3.Text(c, title, material3.TitleMedium)
		} else {
			ui.Row(c.Key("header")).FillWidth().AlignItems(ui.Center).Gap(settingGap(c, 8)).Children(func() {
				material3.Text(c, title, material3.TitleMedium).Grow(1).MinWidth(0)
				action(c.Key("action"))
			})
		}
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
		FocusRingColor(theme.Accent)
}

func AboutRow(c *ui.Context, row ui.Element) ui.Element {
	return material3.CardStyle(row.FillWidth().MinHeight(44).AlignItems(ui.Center).Gap(settingGap(c, 12)), material3.ElevatedCard).PaddingX(settingX(c, 16))
}

func AboutPanel(c *ui.Context, icons asseticon.Set, version func(*ui.Context)) {
	section(c, func() {
		material3.Text(c, locales.AppAbout(), material3.HeadlineMedium)
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
