package theme

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

var (
	Canvas     = material3.Monochrome.Surface
	Side       = material3.Monochrome.SurfaceContainer
	White      = ui.Hex("#FFFFFF")
	Border     = material3.DividerColor()
	Ink        = material3.Monochrome.OnSurface
	Muted      = material3.Monochrome.OnSurfaceVariant
	Accent     = material3.Monochrome.Primary
	CaretColor = ui.Hex("#3e3e3e")
	Tint       = material3.Monochrome.PrimaryContainer
	Selected   = material3.Monochrome.SecondaryContainer
	Hover      = material3.Monochrome.OnSurface.Alpha(.04)
	ErrorFG    = material3.Monochrome.Error
)

func TextInput(c *ui.Context, value *string) ui.Element {
	return ui.TextInputBase(c, value).CaretColor(CaretColor)
}

// InputFrame keeps the horizontal inset clear even when the editor scrolls.
func InputFrame(input ui.Element, inset, radius float32, _ ui.Color) ui.Element {
	return material3.InputFrame(input, inset, max(material3.ShapeSmall, radius))
}

func PointerButton(c *ui.Context, disabled bool) ui.Element {
	// Low-level targets often live inside a row that already paints its state
	// layer, or contain a square icon face. The owning component draws one
	// feedback surface; adding a second automatic layer here darkens it twice.
	button := ui.ButtonBase(c).Disabled(disabled).BorderWidth(0).Radius(material3.ShapeSmall).
		Background(ui.Transparent).TextColor(Ink).NoShadow().FocusRingColor(Accent)
	if !disabled {
		button.Cursor(ui.CursorPointer)
	}
	return button
}

func Switch(c *ui.Context, on *bool, label string, change func()) {
	material3.Switch(c.Key("control"), on, label).OnChange(change)
}
