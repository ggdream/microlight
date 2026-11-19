package settings

import (
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
)

func ActionButton(c *ui.Context, icons asseticon.Set, label, glyph string, disabled bool) ui.Element {
	button := theme.PointerButton(c, disabled).Label(label).MinHeight(SelectHeight(c)).PaddingX(settingX(c, 9)).
		Radius(7).Border(1, theme.Border).Gap(settingGap(c, 5)).AlignItems(ui.Center).Shrink(0)
	color := theme.Accent
	if disabled {
		color = theme.Muted
	} else if button.Hovered() {
		button.Background(theme.Tint)
	}
	return button.Children(func() {
		ui.Icon(c, icons.SVG(glyph)).Size(14, 14).Shrink(0).TextColor(color)
		ui.Text(c, label).FontSize(viewport.Font(c, 12)).TextColor(color)
	})
}
