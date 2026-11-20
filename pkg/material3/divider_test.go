package material3

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDividerIsVisibleAndRespectsInsets(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		tester := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Background(ui.Hex("#FFFFFF")).Padding(10).Children(func() {
				line := Divider(c, DividerOptions{Vertical: vertical, Space: 9, Inset: 12, EndInset: 20})
				if vertical {
					line.Height(100)
				} else {
					line.Width(100)
				}
			})
		}, 140, 140)
		img := tester.Image()
		at := func(along int) uint8 {
			if vertical {
				return img.RGBAAt(14, 10+along).R
			}
			return img.RGBAAt(10+along, 14).R
		}
		if at(30) >= 240 {
			t.Fatalf("divider was not visible (vertical %v)", vertical)
		}
		if at(5) < 250 || at(90) < 250 {
			t.Fatalf("divider painted into its insets (vertical %v)", vertical)
		}
	}
}
