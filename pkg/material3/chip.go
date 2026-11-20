package material3

import "github.com/egoist/mygo/ui"

// ChoiceChipStyle uses the elevated single-select chip hierarchy without an
// outline. The monochrome theme gives the selected chip a high-contrast fill.
// Apply it to the containing row so nested actions share one state layer.
func ChoiceChipStyle(chip ui.Element, selected bool) ui.Element {
	background, elevation := Monochrome.SurfaceHigh, 1
	foreground := ChoiceChipForeground(selected)
	if selected {
		background, elevation = Monochrome.Primary, 0
	}
	opacity := float32(0)
	switch {
	case chip.Pressed(), chip.FocusWithin():
		opacity = .12
	case chip.Hovered():
		opacity = .08
	}
	return Elevate(chip.BorderWidth(0).Radius(ShapeSmall).
		Background(background.Mix(foreground, opacity)).TextColor(foreground), elevation)
}

func ChoiceChipForeground(selected bool) ui.Color {
	if selected {
		return Monochrome.OnPrimary
	}
	return Monochrome.OnSurface
}
