package viewport

import "testing"

func TestAxesAndTypographyRespondIndependently(t *testing.T) {
	base := Layout{Width: 390, Height: 760}
	short := Layout{Width: 390, Height: 360}
	narrow := Layout{Width: 320, Height: 760}
	wide := Layout{Width: 1400, Height: 760}
	if short.Y(20) >= base.Y(20) || short.X(20) != base.X(20) || short.Text(14) != base.Text(14) {
		t.Fatal("a height-only resize should only scale vertical spacing")
	}
	if narrow.X(20) >= base.X(20) || narrow.Text(14) >= base.Text(14) || narrow.Y(20) != base.Y(20) {
		t.Fatal("a width-only resize should scale horizontal spacing and text")
	}
	if short.Target(34) < 44 || narrow.Target(34) < 44 {
		t.Fatal("compact touch targets should stay usable")
	}
	if wide.Text(14) <= base.Text(14) || wide.Y(20) != base.Y(20) {
		t.Fatal("wide windows should use the same independent typography policy")
	}
}
