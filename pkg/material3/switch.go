package material3

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/viewport"
	"time"
)

// Switch keeps MyGo's keyboard, touch and accessibility behavior while drawing
// a Material track and thumb consistently on every platform.
func Switch(c *ui.Context, on *bool, label string) ui.Element {
	layout := viewport.Available(c)
	w, h := layout.X(38), layout.X(22)
	control := ui.SwitchBase(c, on).Label(label).Size(max(layout.Target(44), w), max(layout.Target(40), h)).Shrink(0).Radius(ShapeFull).
		Cursor(ui.CursorPointer).FocusRingColor(Monochrome.Primary)
	target := float32(0)
	if *on {
		target = 1
	}
	progress := control.Animate("thumb", target, 160*time.Millisecond)
	hovered := control.Hovered() || control.Pressed()
	return control.Draw(func(p *ui.Painter, box ui.Rect) {
		track := ui.Rect{X: box.X + (box.W-w)/2, Y: box.Y + (box.H-h)/2, W: w, H: h}
		fill := Monochrome.SurfaceHighest.Mix(Monochrome.Primary, progress)
		p.Fill(track, fill, h/2)
		diameter := h * (.5 + .25*progress)
		cx := track.X + h/2 + (w-h)*progress
		cy := track.Y + h/2
		if hovered {
			p.Fill(ui.Rect{X: cx - h*.6, Y: cy - h*.6, W: h * 1.2, H: h * 1.2}, Monochrome.Primary.Alpha(.08), h*.6)
		}
		thumb := Monochrome.OnSurfaceVariant.Mix(Monochrome.OnPrimary, progress)
		p.Fill(ui.Rect{X: cx - diameter/2, Y: cy - diameter/2, W: diameter, H: diameter}, thumb, diameter/2)
	})
}
