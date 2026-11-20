package material3

import "github.com/egoist/mygo/ui"

// DividerColor is independent of control borders. Built-in menus can use it
// for their own separator rows while controls retain borderless surfaces.
func DividerColor() ui.Color { return Monochrome.OutlineVariant }

type DividerOptions struct {
	Vertical         bool
	Space, Thickness float32
	Inset, EndInset  float32
	Color            ui.Color
}

// Divider separates content groups. Space is the total extent perpendicular
// to the line; Thickness is the centered line itself. Defaults follow Material
// 3's 16-point space and 1-point outlineVariant line. Insets are directional.
func Divider(c *ui.Context, options ...DividerOptions) ui.Element {
	var o DividerOptions
	if len(options) > 0 {
		o = options[0]
	}
	space, thickness := o.Space, o.Thickness
	if space <= 0 {
		space = 16
	}
	if thickness <= 0 {
		thickness = 1
	}
	thickness = min(space, thickness)
	inset, endInset := max(0, o.Inset), max(0, o.EndInset)
	color := o.Color
	if color.A == 0 {
		color = DividerColor()
	}
	rtl := c.Direction() == ui.RightToLeft
	divider := ui.Box(c).Role(ui.RoleSeparator).Shrink(0).IgnoreInput(true)
	if o.Vertical {
		divider.Width(space).FillHeight()
	} else {
		divider.Height(space).FillWidth()
	}
	return divider.Draw(func(p *ui.Painter, box ui.Rect) {
		if o.Vertical {
			box = ui.Rect{X: box.X + (box.W-thickness)/2, Y: box.Y + inset,
				W: thickness, H: max(0, box.H-inset-endInset)}
		} else {
			start := inset
			if rtl {
				start = endInset
			}
			box = ui.Rect{X: box.X + start, Y: box.Y + (box.H-thickness)/2,
				W: max(0, box.W-inset-endInset), H: thickness}
		}
		p.Fill(box, color, 0)
	})
}

type Edge uint8

const (
	TopEdge Edge = iota
	RightEdge
	BottomEdge
	LeftEdge
)

// EdgeDivider separates adjacent regions without adding another layout row
// or surrounding the component with an outline.
func EdgeDivider(element ui.Element, edge Edge) ui.Element {
	return element.Draw(func(p *ui.Painter, box ui.Rect) {
		switch edge {
		case TopEdge:
			box.H = 1
		case BottomEdge:
			box.Y, box.H = box.Y+max(0, box.H-1), 1
		case LeftEdge:
			box.W = 1
		case RightEdge:
			box.X, box.W = box.X+max(0, box.W-1), 1
		default:
			return
		}
		p.Fill(box, DividerColor(), 0)
	})
}
