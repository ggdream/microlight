// Package viewport adapts layouts to the available logical window size.
package viewport

const (
	CompactWidth    = 600
	ExpandedWidth   = 900
	referenceWidth  = 390
	referenceHeight = 760
)

type Layout struct {
	Width, Height float32
}

func (l Layout) Compact() bool  { return l.Width > 0 && l.Width < CompactWidth }
func (l Layout) Expanded() bool { return l.Width >= ExpandedWidth }

// X scales horizontal spacing within the bounds of the design system.
func (l Layout) X(points float32) float32 {
	if l.Width <= 0 {
		return points
	}
	return points * min(1.15, max(.85, l.Width/referenceWidth))
}

// Y scales vertical spacing independently. Short windows give more room to
// content, and the limit prevents the keyboard from flattening the layout.
func (l Layout) Y(points float32) float32 {
	if l.Height <= 0 {
		return points
	}
	return points * min(1.15, max(.75, l.Height/referenceHeight))
}

// Text deliberately follows width only, so opening the keyboard preserves
// type sizes. Use a gentler curve for type than for layout distances.
func (l Layout) Text(points float32) float32 {
	if l.Width <= 0 {
		return points
	}
	ratio := l.Width / referenceWidth
	sensitivity := float32(.03)
	if ratio < 1 {
		sensitivity = .28
	}
	return points * min(1.10, max(.95, 1+(ratio-1)*sensitivity))
}

// Target scales control height and preserves the minimum compact hit area.
func (l Layout) Target(points float32) float32 {
	if l.Compact() {
		return max(44, l.Y(points))
	}
	return l.Y(points)
}

func (l Layout) Gutter(desktop float32) float32 {
	if l.Compact() {
		return l.X(12)
	}
	return desktop
}

// Density chooses spacing tokens for the current layout.
func (l Layout) Density(regular, compact float32) float32 {
	if l.Compact() {
		return compact
	}
	return regular
}

// FitWidth limits a floating panel to the available window and edge insets.
func (l Layout) FitWidth(preferred, inset float32) float32 {
	if l.Width <= 0 {
		return preferred
	}
	return min(preferred, max(0, l.Width-2*inset))
}

// DialogWidth keeps centered phone dialogs distinct from full-screen surfaces.
// Preserve readable forms on small screens and avoid stretching on wider ones.
func (l Layout) DialogWidth() float32 {
	return l.FitWidth(min(360, l.Width*.80), l.X(24))
}

// FitHeight leaves space above and below centered content.
func (l Layout) FitHeight(preferred, inset float32) float32 {
	if l.Height <= 0 {
		return preferred
	}
	return min(preferred, max(0, l.Height-2*inset))
}
