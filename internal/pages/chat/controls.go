package chat

import (
	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func quietButton(c *ui.Context, label, glyph string) ui.Element {
	button := pointerButton(c, false).Label(label).Gap(5).AlignItems(ui.Center).Padding(4, 7).Radius(6)
	if button.Hovered() {
		button.Background(side)
	}
	return button.Children(func() {
		icon(c, glyph, muted, 13)
		ui.Text(c, label).FontSize(11).TextColor(muted)
	})
}

func actionButton(c *ui.Context, label, glyph string, disabled bool) ui.Element {
	bg, fg := accent, white
	if disabled {
		bg, fg = side, muted
	}
	return pointerButton(c, disabled).Label(label).Tooltip(label).Width(34).Height(34).Radius(17).
		Background(bg).AlignItems(ui.Center).Justify(ui.Center).
		Children(func() { icon(c, glyph, fg, 18) })
}

func stopButton(c *ui.Context) ui.Element {
	label := locales.Stop()
	button := pointerButton(c, false).Label(label).Tooltip(label).Size(replyControlHeight, replyControlHeight).Shrink(0).
		Radius(replyControlRadius).AlignItems(ui.Center).Justify(ui.Center)
	background := ui.Hex("#FCEDEE")
	if button.Hovered() {
		background = ui.Hex("#F8DBDE")
	}
	return button.Background(background).Children(func() {
		icon(c, "stop", errorFG, 16)
	})
}

func drawSpark(color ui.Color) func(*ui.Painter, ui.Rect) {
	return func(p *ui.Painter, r ui.Rect) {
		x, y := r.X+r.W/2, r.Y+r.H/2
		a := min(r.W, r.H) * .24
		var star ui.Path
		star.MoveTo(x, y-a).QuadTo(x+a*.16, y-a*.16, x+a, y).
			QuadTo(x+a*.16, y+a*.16, x, y+a).
			QuadTo(x-a*.16, y+a*.16, x-a, y).
			QuadTo(x-a*.16, y-a*.16, x, y-a).Close()
		p.FillPath(&star, color)
	}
}
