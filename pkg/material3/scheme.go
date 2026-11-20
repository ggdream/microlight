// Package material3 provides native MyGo components with Material 3 color
// roles, surface elevation, shapes and interaction states. It has no platform
// bindings; the host retains navigation, responsive sizing and behavior.
package material3

import "github.com/egoist/mygo/ui"

// Scheme separates a component's role from the particular application palette.
type Scheme struct {
	Primary, OnPrimary, PrimaryContainer, OnPrimaryContainer                          ui.Color
	SecondaryContainer                                                                ui.Color
	Surface, SurfaceBright, SurfaceLow, SurfaceContainer, SurfaceHigh, SurfaceHighest ui.Color
	OnSurface, OnSurfaceVariant, Outline, OutlineVariant                              ui.Color
	Error, OnError, ErrorContainer, OnErrorContainer                                  ui.Color
	Scrim, Shadow                                                                     ui.Color
}

// Monochrome uses black as the primary color on neutral, light surfaces.
var Monochrome = Scheme{
	Primary: ui.Hex("#101010"), OnPrimary: ui.Hex("#FFFFFF"),
	PrimaryContainer: ui.Hex("#EFEFEF"), OnPrimaryContainer: ui.Hex("#1B1B1B"),
	SecondaryContainer: ui.Hex("#E6E6E6"),
	Surface:            ui.Hex("#FAFAFA"), SurfaceBright: ui.Hex("#FFFFFF"), SurfaceLow: ui.Hex("#FCFCFC"),
	SurfaceContainer: ui.Hex("#F5F5F5"), SurfaceHigh: ui.Hex("#EFEFEF"), SurfaceHighest: ui.Hex("#E9E9E9"),
	OnSurface: ui.Hex("#1B1B1B"), OnSurfaceVariant: ui.Hex("#626262"), Outline: ui.Hex("#858585"), OutlineVariant: ui.Hex("#D5D5D5"),
	Error: ui.Hex("#B3261E"), OnError: ui.Hex("#FFFFFF"),
	ErrorContainer: ui.Hex("#F9DEDC"), OnErrorContainer: ui.Hex("#410E0B"),
	Scrim: ui.Hex("#000000"), Shadow: ui.Hex("#000000"),
}

const (
	ShapeSmall      float32 = 8
	ShapeMedium     float32 = 12
	ShapeLarge      float32 = 16
	ShapeExtraLarge float32 = 28
	ShapeFull       float32 = 999
)

// Theme also reaches MyGo's built-in checkboxes, switches, menus, pickers,
// selection, scrollbars and Markdown. Borderless controls use filled surfaces;
// OutlineVariant is retained for content dividers, never a control perimeter.
func (s Scheme) Theme() *ui.Theme {
	t := ui.LightTheme()
	t.Background, t.Surface = s.Surface, s.SurfaceContainer
	t.SurfaceHover, t.SurfacePressed = s.SurfaceLow.Mix(s.OnSurface, .04), s.SurfaceLow.Mix(s.OnSurface, .08)
	t.Border = ui.Transparent
	t.Text, t.TextMuted = s.OnSurface, s.OnSurfaceVariant
	t.Accent, t.InactiveAccent, t.AccentText = s.Primary, s.Primary, s.OnPrimary
	t.AccentHover, t.AccentPressed = ui.Hex("#2B2B2B"), ui.Hex("#383838")
	t.Focus, t.Selection = s.Primary, s.OnSurface.Alpha(.18)
	t.FocusRingWidth, t.FocusRingOffset = 2, 2
	t.Danger, t.Inverse, t.InverseText = s.Error, s.OnSurface, s.Surface
	t.Scrollbar, t.ScrollbarWidth = s.OnSurfaceVariant.Alpha(.38), 4
	t.Radius, t.FontSize = ShapeMedium, 13
	return t
}
