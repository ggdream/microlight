package theme

import "github.com/egoist/mygo/ui"

var (
	Canvas     = ui.Hex("#FCFDFA")
	Side       = ui.Hex("#F1F4EF")
	White      = ui.Hex("#FFFFFF")
	Border     = ui.Hex("#DFE6DF")
	Ink        = ui.Hex("#212C27")
	Muted      = ui.Hex("#5E6C63")
	Accent     = ui.Hex("#16654F")
	CaretColor = ui.Hex("#3e3e3e")
	Tint       = ui.Hex("#E3F0E8")
	Hover      = ui.Hex("#E7ECE6")
	ErrorFG    = ui.Hex("#9E3F44")
)

func TextInput(c *ui.Context, value *string) ui.Element {
	return ui.TextInputBase(c, value).CaretColor(CaretColor)
}

// InputFrame keeps the horizontal inset clear even when the editor scrolls.
func InputFrame(input ui.Element, inset, radius float32, border ui.Color) ui.Element {
	// MyGo clips editor ink at the border, so reserve the inset there and draw the visible outline separately.
	return input.BorderWidth(1, inset+1).BorderColor(ui.Color{}).Radius(radius).
		Draw(func(p *ui.Painter, box ui.Rect) { p.Stroke(box, border, radius, 1) })
}

func PointerButton(c *ui.Context, disabled bool) ui.Element {
	button := ui.ButtonBase(c).Disabled(disabled)
	if !disabled {
		button.Cursor(ui.CursorPointer)
	}
	return button
}

func Switch(c *ui.Context, on *bool, label string, change func()) {
	switchTheme := *c.Theme()
	switchTheme.Accent = Accent
	switchTheme.InactiveAccent = Accent
	ui.Themed(c, &switchTheme).Background(ui.Transparent).Shrink(0).Children(func() {
		ui.Switch(c.Key("control"), on).Label(label).OnChange(change)
	})
}
