package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

func Section(c *ui.Context, title, subtitle string, content func()) {
	section(c, func() {
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, title).FontSize(29).FontWeight(680)
			ui.Text(c, subtitle).FontSize(13).TextColor(theme.Muted)
		})
	}, content)
}

func section(c *ui.Context, header, content func()) {
	scroll, offset := elastic.Scroll(c)
	scroll.Grow(1).MinHeight(0).Children(func() {
		offset.Apply(ui.Column(c)).FillWidth().AlignItems(ui.Center).Padding(26, 22).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(610).Gap(16).Children(func() {
				header()
				content()
			})
		})
	})
}

func Footer(c *ui.Context, message string, failed bool, save func()) {
	ui.Column(c).Shrink(0).AlignItems(ui.Center).Padding(11, 22).
		Background(theme.Canvas).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().MaxWidth(610).AlignItems(ui.Center).Gap(12).Children(func() {
			color := theme.Accent
			if failed {
				color = theme.ErrorFG
			}
			ui.Text(c, message).Grow(1).MinWidth(0).FontSize(12).TextColor(color)
			button := theme.PointerButton(c, false).Label(locales.SaveSettings()).Height(36).Shrink(0).PaddingX(15).
				Radius(8).Background(theme.Accent).AlignItems(ui.Center)
			if button.Hovered() {
				button.Background(ui.Hex("#115541"))
			}
			button.Children(func() {
				ui.Text(c, locales.SaveSettings()).FontSize(13).FontWeight(620).TextColor(theme.White)
			}).OnClick(save)
		})
	})
}

func Card(c *ui.Context, title string, fields func()) ui.Element {
	return ui.Column(c).FillWidth().Background(theme.White).Radius(13).Border(1, theme.Border).
		Padding(15, 16).Gap(11).Children(func() {
		ui.Text(c, title).FontSize(15).FontWeight(660)
		fields()
	})
}

func Field(c *ui.Context, label, placeholder string, value *string) ui.Element {
	var input ui.Element
	ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(610)
		input = TextField(c.Key(label), label, placeholder, value).FillWidth()
	})
	return input
}

func TextField(c *ui.Context, label, placeholder string, value *string) ui.Element {
	return theme.TextInput(c, value).Label(label).Placeholder(placeholder).
		Height(35).PaddingX(10).FontSize(13).
		Background(theme.Canvas).Border(1, theme.Border).Radius(7).FocusRingColor(theme.Accent)
}

func Version(c *ui.Context, version string) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).
		Background(theme.White).Radius(13).Border(1, theme.Border).Padding(12, 16).Children(func() {
		ui.Text(c, locales.AppVersion()).Grow(1).MinWidth(0).FontSize(15).FontWeight(660)
		ui.Text(c, version).Font("monospace").FontSize(13).TextColor(theme.Muted).Selectable()
	})
}

func About(c *ui.Context, icons asseticon.Set) {
	button := theme.PointerButton(c, false).Label(locales.AppAbout()).
		FillWidth().AlignItems(ui.Center).Gap(12).
		Background(theme.White).Radius(13).Border(1, theme.Border).Padding(12, 16).
		FocusRingColor(theme.Accent)
	if button.Hovered() {
		button.Background(theme.Hover)
	}
	button.Children(func() {
		ui.Text(c, locales.AppAbout()).Grow(1).MinWidth(0).FontSize(15).FontWeight(660)
		ui.Row(c).AlignItems(ui.Center).Gap(7).Shrink(0).Children(func() {
			ui.Icon(c, icons.SVG("github")).Size(16, 16).TextColor(theme.Ink)
			ui.Text(c, "GitHub").FontSize(13).TextColor(theme.Ink)
			ui.Icon(c, icons.SVG("open")).Size(14, 14).TextColor(theme.Muted)
		})
	}).OnClick(func() {
		c.OpenURL("https://github.com/ggdream/microlight")
	})
}
