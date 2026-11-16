package theme

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestInputFrameClipsLongValuesInsideBothInsets(t *testing.T) {
	for _, name := range []string{"text", "password", "multiline"} {
		t.Run(name, func(t *testing.T) {
			value := strings.Repeat("W", 100)
			tester := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Padding(10).Background(White).Children(func() {
					input := ui.TextInputBase
					if name == "multiline" {
						input = ui.TextAreaBase
					}
					field := InputFrame(input(c, &value), 10, 6, Border).
						Label("Example").Width(120).Height(35).Background(White).
						TextColor(Ink).FontSize(13).FocusRing(false)
					if name == "password" {
						field.Password()
					}
				})
			}, 150, 60)

			assertInsets := func() {
				t.Helper()
				img := tester.Image()
				contentPainted := false
				for y := 13; y < 42; y++ {
					for x := 13; x < 127; x++ {
						if img.RGBAAt(x, y).R > 160 {
							continue
						}
						if x < 21 || x >= 119 {
							t.Fatalf("editor painted inside a side inset at (%d, %d)", x, y)
						}
						contentPainted = true
					}
				}
				if !contentPainted {
					t.Fatal("editor content was not painted")
				}
			}
			assertInsets()
			if err := tester.Click("Example"); err != nil {
				t.Fatal(err)
			}
			tester.Key(0, ui.KeyEnd)
			assertInsets()
		})
	}
}
