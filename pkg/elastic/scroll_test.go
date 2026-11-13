package elastic

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestViewportEdgeAndRecovery(t *testing.T) {
	for _, kind := range []string{"vertical", "horizontal", "list"} {
		t.Run(kind, func(t *testing.T) {
			var list ui.ListState
			var offset Offset
			tester := ui.NewTester(func(c *ui.Context) {
				if kind == "list" {
					List(c, &list, 10, func(i int, displacement Offset) {
						offset = displacement
						displacement.Apply(ui.Text(c, fmt.Sprintf("Item %d", i))).Height(30)
					}).Fill()
					return
				}
				var view ui.Element
				if kind == "horizontal" {
					view, offset = Horizontal(c)
				} else {
					view, offset = Scroll(c)
				}
				view.Fill().Children(func() {
					content := ui.Column(c).Shrink(0)
					if kind == "horizontal" {
						content.Width(300)
					} else {
						content.Height(300)
					}
					offset.Apply(content).Children(func() { ui.Text(c, "Item 0") })
				})
			}, 120, 120)
			tester.Move(50, 50)
			dx, dy := float32(0), float32(-60)
			if kind == "horizontal" {
				dx, dy = dy, dx
			}
			tester.Scroll(50, 50, dx, dy)
			tester.Frame()
			at := func() float32 {
				if kind == "horizontal" {
					return offset.X
				}
				return offset.Y
			}
			initial := at()
			if initial <= 1 || initial >= 60 {
				t.Fatalf("expected resisted edge movement, got %+v", offset)
			}
			tester.Advance(130 * time.Millisecond)
			tester.Frame()
			if value := at(); value <= 0 || value >= initial {
				t.Fatalf("expected damped return from %v, got %v", initial, value)
			}
			tester.Advance(time.Second)
			tester.Frame()
			if at() != 0 {
				t.Fatalf("expected zero after settling, got %+v", offset)
			}
			tester.Scroll(50, 50, -dx/2, -dy/2)
			tester.Frame()
			if at() != 0 {
				t.Fatalf("normal scrolling triggered the edge effect: %+v", offset)
			}
			tester.Scroll(50, 50, -dx*8, -dy*8)
			tester.Frame()
			if at() >= -1 || at() <= -110 {
				t.Fatalf("expected resisted movement at the far edge, got %+v", offset)
			}
			far := at()
			tester.Scroll(50, 50, dx/6, dy/6)
			tester.Frame()
			if value := at(); value <= far || value >= 0 {
				t.Fatalf("expected a partial reversal from %v, got %v", far, value)
			}
			tester.Scroll(50, 50, dx*4, dy*4)
			tester.Frame()
			if at() != 0 {
				t.Fatalf("expected reversing to clear the edge effect, got %+v", offset)
			}
			tester.SetPreferences(ui.Preferences{ReduceMotion: true})
			tester.Scroll(50, 50, -dx*8, -dy*8)
			tester.Frame()
			if at() != 0 {
				t.Fatalf("reduced motion triggered the edge effect: %+v", offset)
			}
		})
	}
}

func TestHorizontalGestureReachesNestedNativeScroll(t *testing.T) {
	var native ui.ScrollState
	var offset Offset
	tester := ui.NewTester(func(c *ui.Context) {
		view, displacement := Scroll(c)
		offset = displacement
		view.Fill().Children(func() {
			offset.Apply(ui.Column(c)).Height(300).Shrink(0).Children(func() {
				ui.ScrollHorizontal(c).Width(120).Height(80).TrackScroll(&native).Children(func() {
					ui.Box(c).Width(300).Height(80).Shrink(0)
				})
			})
		})
	}, 120, 120)
	tester.Move(50, 40)
	tester.Scroll(50, 40, 40, -3)
	tester.Frame()
	if native.X != 40 || offset != (Offset{}) {
		t.Fatalf("horizontal gesture should scroll only the inner viewport, got X=%v and offset %+v", native.X, offset)
	}
}

func TestMomentumHandsOffAtEdge(t *testing.T) {
	now := time.Now()
	var a axis
	offset, handled := a.move(now, 80, 100, 35, 120, ui.InputEvent{Precise: true, Momentum: true})
	initial := a.value(now)
	if !handled || offset != 100 || initial <= 0 {
		t.Fatalf("expected momentum to reach the edge and stretch it, got offset %v and pull %v", offset, initial)
	}
	later := now.Add(16 * time.Millisecond)
	before := a.value(later)
	if before <= initial {
		t.Fatalf("expected spring to retain incoming velocity, got %v after %v", before, initial)
	}
	a.move(later, offset, 100, 12, 120, ui.InputEvent{Precise: true, Momentum: true})
	if after := a.value(later); after != before || a.release != now {
		t.Fatalf("outward momentum restarted the spring: pull %v after %v, release %v", after, before, a.release)
	}
	if end := a.value(now.Add(time.Second)); end != 0 {
		t.Fatalf("expected spring to settle, got %v", end)
	}
}
