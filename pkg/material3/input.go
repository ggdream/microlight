package material3

import "github.com/egoist/mygo/ui"

// InputFrame reserves both insets in the editor's own clipping bounds, even
// for scrolled password fields. The transparent layout border reserves clipping
// insets; a separately painted outline keeps focus from moving the content.
// This styles form editors, not the chat composer.
func InputFrame(input ui.Element, inset, radius float32) ui.Element {
	stroke, thickness := Monochrome.Outline, float32(1)
	switch {
	case input.Focused():
		stroke, thickness = Monochrome.Primary, 2
	case input.Hovered():
		stroke = Monochrome.OnSurface
	}
	return input.NoShadow().BorderWidth(1, inset+1).BorderColor(ui.Transparent).Radius(radius).
		Background(Monochrome.SurfaceBright).TextColor(Monochrome.OnSurface).CaretColor(ui.Hex("#3e3e3e")).
		FocusRing(false).Draw(func(p *ui.Painter, box ui.Rect) {
		p.Stroke(box, stroke, radius, thickness)
	})
}

func SelectFrame(trigger ui.Element) ui.Element {
	color := Monochrome.SurfaceContainer
	if trigger.Hovered() || trigger.Pressed() {
		color = Monochrome.SurfaceHigh
	}
	return trigger.BorderWidth(0).Radius(ShapeSmall).Background(color).FocusRingColor(Monochrome.Primary)
}
