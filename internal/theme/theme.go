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

func PointerButton(c *ui.Context, disabled bool) ui.Element {
	button := ui.ButtonBase(c).Disabled(disabled)
	if !disabled {
		button.Cursor(ui.CursorPointer)
	}
	return button
}

var checkboxTheme = func() *ui.Theme {
	t := ui.LightTheme().WithAccent(Accent)
	t.Background, t.Border, t.Text, t.AccentText = White, Border, Ink, White
	return t
}()

func Checkbox(c *ui.Context, checked *bool, label string, disabled bool) {
	ui.Themed(c, checkboxTheme).Background(ui.Transparent).Children(func() {
		ui.Checkbox(c.Key("checkbox"), checked, label).Disabled(disabled)
	})
}
