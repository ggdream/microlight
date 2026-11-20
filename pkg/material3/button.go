package material3

import "github.com/egoist/mygo/ui"

type ButtonVariant uint8

const (
	ElevatedButton ButtonVariant = iota
	FilledButton
	TonalButton
	TextButton
)

func Button(c *ui.Context, label string, variant ButtonVariant, disabled bool) ui.Element {
	button := ui.ButtonBase(c).Label(label).Disabled(disabled).AlignItems(ui.Center).
		Justify(ui.Center).Gap(8).PaddingX(16).Height(36).Shrink(0).FocusRingColor(Monochrome.Primary)
	if !disabled {
		button.Cursor(ui.CursorPointer)
	}
	return ButtonStyle(button, variant, disabled)
}

func Foreground(variant ButtonVariant, disabled bool) ui.Color {
	if disabled {
		return Monochrome.OnSurface.Alpha(.38)
	}
	if variant == FilledButton {
		return Monochrome.OnPrimary
	}
	return Monochrome.Primary
}

func ButtonStyle(button ui.Element, variant ButtonVariant, disabled bool) ui.Element {
	color, foreground, elevation := ui.Transparent, Foreground(variant, disabled), 0
	switch variant {
	case ElevatedButton:
		color, elevation = Monochrome.SurfaceLow, 1
	case FilledButton:
		color = Monochrome.Primary
	case TonalButton:
		color = Monochrome.PrimaryContainer
	case TextButton:
	}
	if disabled {
		if variant != TextButton {
			color = Monochrome.OnSurface.Alpha(.12)
		}
		elevation = 0
	} else {
		opacity := float32(0)
		switch {
		case button.Pressed(), button.FocusVisible():
			opacity = .12
		case button.Hovered():
			opacity = .04
		}
		if opacity > 0 {
			if color.A == 0 {
				color = foreground.Alpha(opacity)
			} else {
				color = color.Mix(foreground, opacity)
			}
		}
		if variant == ElevatedButton && button.Hovered() && !button.Pressed() {
			elevation = 3
		}
	}
	// Disabled colors already include Material's opacity; prevent the engine
	// from dimming the whole subtree a second time.
	return Elevate(button.DimDisabled(false).BorderWidth(0).Radius(ShapeSmall).Background(color).TextColor(foreground), elevation)
}
