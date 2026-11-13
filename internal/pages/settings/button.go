package settings

import (
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
)

func ActionButton(c *ui.Context, icons asseticon.Set, label, glyph string, disabled bool) ui.Element {
	button := theme.PointerButton(c, disabled).Label(label).Height(33).PaddingX(9).
		Radius(7).Border(1, theme.Border).Gap(5).AlignItems(ui.Center).Shrink(0)
	color := theme.Accent
	if disabled {
		color = theme.Muted
	} else if button.Hovered() {
		button.Background(theme.Tint)
	}
	return button.Children(func() {
		ui.Icon(c, icons.SVG(glyph)).Size(14, 14).Shrink(0).TextColor(color)
		ui.Text(c, label).FontSize(12).TextColor(color)
	})
}
