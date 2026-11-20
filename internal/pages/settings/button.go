package settings

import (
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

func ActionButton(c *ui.Context, icons asseticon.Set, label, glyph string, disabled bool, variants ...material3.ButtonVariant) ui.Element {
	variant := material3.FilledButton
	if len(variants) > 0 {
		variant = variants[0]
	}
	button := material3.Button(c, label, variant, disabled).Height(SelectHeight(c)).PaddingX(settingX(c, 12)).Gap(settingGap(c, 6))
	color := material3.Foreground(variant, disabled)
	return button.Children(func() {
		ui.Icon(c, icons.SVG(glyph)).Size(14, 14).Shrink(0).TextColor(color)
		ui.Text(c, label).FontSize(viewport.Font(c, 12)).TextColor(color)
	})
}
