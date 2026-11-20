package chat

import (
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
)

func quietButton(c *ui.Context, label, glyph string) ui.Element {
	button := material3.Button(c, label, material3.TextButton, false).Height(windowLayout(c).Target(28)).Gap(5).PaddingX(9)
	return button.Children(func() {
		icon(c, glyph, muted, 13)
		ui.Text(c, label).FontSize(viewport.Font(c, 11)).TextColor(muted)
	})
}

func actionButton(c *ui.Context, label, glyph string, disabled bool) ui.Element {
	return material3.Button(c, label, material3.FilledButton, disabled).Tooltip(label).Size(34, 34).PaddingX(0).
		Children(func() { icon(c, glyph, material3.Foreground(material3.FilledButton, disabled), 18) })
}

func stopButton(c *ui.Context) ui.Element {
	label := locales.Stop()
	size := windowLayout(c).Target(replyControlHeight)
	button := pointerButton(c, false).Label(label).Tooltip(label).Size(size, size).Shrink(0).
		Radius(replyControlRadius).AlignItems(ui.Center).Justify(ui.Center)
	background := material3.Monochrome.ErrorContainer
	if button.Hovered() {
		background = background.Mix(errorFG, .08)
	}
	return button.Background(background).Children(func() {
		icon(c, "stop", errorFG, 16)
	})
}
