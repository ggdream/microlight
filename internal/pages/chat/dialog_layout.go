package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

// Keep the actions visible while long dialog content scrolls above them.
func dialogBody(c *ui.Context, content func()) {
	scroll, offset := elastic.Scroll(c.Key("dialog-body"))
	scroll.FillWidth().MinHeight(0).Shrink(1).Children(func() {
		offset.Apply(ui.Column(c)).FillWidth().Gap(windowLayout(c).Density(12, 8)).Children(content)
	})
}

func dialogActions(c *ui.Context) ui.Element {
	return ui.Row(c).FillWidth().Shrink(0).Wrap().Justify(ui.End).Gap(windowLayout(c).Density(8, 6))
}

func dialogMaxHeight(c *ui.Context) float32 {
	layout := windowLayout(c)
	return layout.FitHeight(layout.Height, 10)
}

func insetDialog(c *ui.Context, backdrop ui.Element) ui.Element {
	top, right, bottom, left := c.SafeArea()
	return backdrop.Padding(top, right, bottom, left)
}

func mobileDialog(c *ui.Context, backdrop, panel ui.Element) {
	layout := windowLayout(c)
	if !layout.Compact() {
		return
	}
	top, right, bottom, left := c.SafeArea()
	backdrop.Justify(ui.Center).AlignItems(ui.Center).
		Padding(top+layout.Y(12), right+layout.X(24), bottom+layout.Y(12), left+layout.X(24))
	panel.Width(layout.DialogWidth()).MaxWidthPercent(100).
		Padding(layout.Y(16), layout.X(16)).Radius(16)
}
