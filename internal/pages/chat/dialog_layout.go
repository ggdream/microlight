package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
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
	material3.Dialog(panel)
	layout := windowLayout(c)
	if !layout.Compact() {
		return
	}
	top, right, bottom, left := c.SafeArea()
	backdrop.Justify(ui.Center).AlignItems(ui.Center).
		Padding(top+layout.Y(12), right+layout.X(24), bottom+layout.Y(12), left+layout.X(24))
	panel.Width(layout.DialogWidth()).MaxWidthPercent(100).
		Padding(layout.Y(16), layout.X(16)).Radius(material3.DialogRadius)
}

// Selection sheets use the existing modal/focus machinery and remain Go UI.
// Unlike ordinary confirmation dialogs, they align to the lower safe edge.
func selectionSheet(c *ui.Context, open *bool, label string, fraction float32, content func()) {
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		layout := windowLayout(c)
		top, right, bottom, left := c.SafeArea()
		backdrop.Justify(ui.End).AlignItems(ui.Stretch).Padding(top, right, 0, left).Background(ink.Alpha(.28))
		material3.Elevate(panel.Label(label).FillWidth().Height(min(layout.Height*fraction+bottom, layout.Height)).MaxHeightPercent(100).
			Background(material3.Monochrome.SurfaceLow).Radius(material3.ShapeExtraLarge, material3.ShapeExtraLarge, 0, 0).Padding(0, 0, bottom, 0), 3)
		ui.Box(c).Width(34).Height(4).Radius(2).AlignSelf(ui.Center).Margin(9, 0, 3).Background(muted.Alpha(.3))
		ui.Column(c).Grow(1).MinHeight(0).Children(content)
	})
}
