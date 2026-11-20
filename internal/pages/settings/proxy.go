package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/proxy"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

func Proxy(c *ui.Context, icons asseticon.Set, config *proxy.Config, validationError string, onChange func(), openSystemSettings func(*ui.Context)) {
	selected := config.EffectiveMode()
	labels := map[proxy.Mode]string{
		proxy.System: locales.ProxySystem(),
		proxy.Direct: locales.ProxyDirect(),
		proxy.Custom: locales.ProxyCustom(),
	}
	Card(c, locales.ProxyNetwork(), func() {
		label := locales.ProxyMode()
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(settingGap(c, 8)).Children(func() {
			ui.Text(c, label).FontSize(viewport.Font(c, 12)).FontWeight(610)
			selectMode := ui.SelectBase(c.Key("proxy-mode"), &selected)
			material3.SelectFrame(selectMode.Trigger.Label(label).Value(labels[selected]).Width(settingsLayout(c).Density(180, 148)).MaxWidthPercent(100).Height(SelectHeight(c)).PaddingX(settingX(c, 10)).
				AlignItems(ui.Center).Justify(ui.SpaceBetween)).Radius(material3.ShapeSmall).
				Background(material3.Monochrome.SurfaceContainer).Cursor(ui.CursorPointer).FocusRingColor(theme.Accent).Children(func() {
				ui.Text(c, labels[selected]).FontSize(viewport.Font(c, 13)).TextColor(theme.Ink)
				ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Rotate(90).TextColor(theme.Muted)
			}).OnChange(func() {
				config.Mode = selected
				onChange()
			})
			selectMode.Popup(func(panel ui.Element) {
				material3.Menu(panel.Padding(settingY(c, 4), settingX(c, 4)).Gap(settingGap(c, 2))).Radius(material3.ShapeMedium)
				for _, mode := range []proxy.Mode{proxy.System, proxy.Direct, proxy.Custom} {
					item := selectMode.Item(mode).Label(labels[mode]).Padding(settingY(c, 6), settingX(c, 8)).Radius(5).Cursor(ui.CursorPointer)
					if item.Highlighted() {
						item.Background(theme.Hover)
					}
					item.Children(func() { ui.Text(c, labels[mode]).FontSize(viewport.Font(c, 13)) })
				}
			})
		})
		if selected == proxy.Custom {
			Field(c, locales.ProxyAddress(), "http://127.0.0.1:7890", &config.URL).
				Password().Keyboard(ui.KeyboardURL).Capitalize(ui.CapitalizeNone).Autocorrect(false).OnChange(onChange)
			ui.Text(c, locales.ProxyCustomHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
		}
		if selected == proxy.System {
			ui.Text(c, locales.ProxySystemHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
			openSystemSettings(c)
		}
		if validationError != "" {
			ui.Text(c, validationError).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
		}
	})
}
