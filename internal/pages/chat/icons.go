package chat

import (
	assets "github.com/ggdream/microlight/internal/icon"

	"github.com/egoist/mygo/ui"
)

var icons assets.Set

func icon(c *ui.Context, glyph string, color ui.Color, size float32) {
	ui.Icon(c, icons.SVG(glyph)).Size(size, size).Shrink(0).TextColor(color)
}
