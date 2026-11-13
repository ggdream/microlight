package chat

import (
	"testing"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func TestSidebarSnapsWhileDraggingAndCanReopen(t *testing.T) {
	app := &chatApp{backend: &Chat{}}
	window := ui.NewTester(app.view, 900, 640)
	dividerAt := func(want float32) {
		t.Helper()
		divider, found := window.Find(locales.ResizeSidebar())
		if !found || divider.X+3 != want {
			t.Fatalf("divider at %+v, want width %v", divider, want)
		}
	}
	dividerAt(defaultSidebarWidth)
	window.Press(defaultSidebarWidth, 180)
	window.Move(120, 180)
	dividerAt(minSidebarWidth)
	if _, shown := window.Find(locales.ExpandSidebar()); !shown {
		t.Fatal("sidebar did not show the compact rail while dragging")
	}
	window.Move(170, 180)
	dividerAt(170)
	window.Move(120, 180)
	window.Release(120, 180)
	dividerAt(minSidebarWidth)

	window.Press(minSidebarWidth, 180)
	window.Move(180, 180)
	window.Release(180, 180)
	dividerAt(180)
}
