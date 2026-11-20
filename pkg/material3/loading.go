package material3

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/viewport"
)

// LoadingOverlay blocks interaction with the underlying view until its caller
// stops rendering it. Spinner animation and progress accessibility use MyGo.
func LoadingOverlay(c *ui.Context, label string) {
	layout := viewport.Available(c)
	ui.Overlay(c, func() {
		ui.Box(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).
			Center().Modal().Role(ui.RoleDialog).Label(label).
			Background(Monochrome.Scrim.Alpha(.20)).Children(func() {
			Elevate(ui.Column(c).AlignItems(ui.Center).Gap(layout.Y(12)).
				MinWidth(layout.X(124)).MaxWidthPercent(80).Padding(layout.Y(22), layout.X(24)).
				Radius(ShapeMedium).Background(Monochrome.Primary), 3).Children(func() {
				ui.Spinner(c).Label(label).Size(layout.X(28), layout.X(28)).TextColor(Monochrome.OnPrimary)
				ui.Text(c, label).FontSize(layout.Text(13)).TextColor(Monochrome.OnPrimary)
			})
		})
	})
}
