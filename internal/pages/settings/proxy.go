package settings

import (
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/proxy"

	"github.com/egoist/mygo/ui"
)

func Proxy(c *ui.Context, icons asseticon.Set, config *proxy.Config, openSystemSettings func(*ui.Context)) {
	selected := config.EffectiveMode()
	labels := map[proxy.Mode]string{
		proxy.System: locales.ProxySystem(),
		proxy.Direct: locales.ProxyDirect(),
		proxy.Custom: locales.ProxyCustom(),
	}
	Card(c, locales.ProxyNetwork(), func() {
		label := locales.ProxyMode()
		ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(10).Children(func() {
			ui.Text(c, label).FontSize(12).FontWeight(610)
			selectMode := ui.SelectBase(c.Key("proxy-mode"), &selected)
			selectMode.Trigger.Label(label).Value(labels[selected]).Width(180).Height(35).PaddingX(10).
				AlignItems(ui.Center).Justify(ui.SpaceBetween).Border(1, theme.Border).Radius(7).
				Background(theme.Canvas).Cursor(ui.CursorPointer).FocusRingColor(theme.Accent).Children(func() {
				ui.Text(c, labels[selected]).FontSize(13).TextColor(theme.Ink)
				ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Rotate(90).TextColor(theme.Muted)
			}).OnChange(func() {
				config.Mode = selected
			})
			selectMode.Popup(func(panel ui.Element) {
				panel.Padding(4).Gap(2).Background(theme.White).Border(1, theme.Border).Radius(8)
				for _, mode := range []proxy.Mode{proxy.System, proxy.Direct, proxy.Custom} {
					item := selectMode.Item(mode).Label(labels[mode]).Padding(6, 8).Radius(5).Cursor(ui.CursorPointer)
					if item.Highlighted() {
						item.Background(theme.Tint)
					}
					item.Children(func() { ui.Text(c, labels[mode]).FontSize(13) })
				}
			})
		})
		if selected == proxy.Custom {
			Field(c, locales.ProxyAddress(), "http://127.0.0.1:7890", &config.URL).
				Password()
			ui.Text(c, locales.ProxyCustomHint()).FontSize(12).TextColor(theme.Muted)
		}
		if selected == proxy.System {
			ui.Text(c, locales.ProxySystemHint()).FontSize(12).TextColor(theme.Muted)
			openSystemSettings(c)
		}
	})
}
