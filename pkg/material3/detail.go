package material3

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/viewport"
)

// DetailField presents a secondary label above its value, allowing long values
// to wrap without squeezing a fixed table column.
func DetailField(c *ui.Context, label string, content func()) ui.Element {
	layout := viewport.Available(c)
	return ui.Column(c).FillWidth().MinWidth(0).Gap(layout.Y(5)).Children(func() {
		ui.Text(c, label).FontSize(layout.Text(11)).TextColor(Monochrome.OnSurfaceVariant)
		content()
	})
}
