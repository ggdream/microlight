package material3

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLoadingOverlayBlocksUnderlyingControlsUntilFinished(t *testing.T) {
	loading, clicks, value := false, 0, "original"
	window := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(Monochrome.Theme())
		ui.Column(c).Children(func() {
			ui.Button(c, "Background action").OnClick(func() { clicks++ })
			ui.TextInput(c, &value).Label("Background field").Width(180)
		})
		if loading {
			LoadingOverlay(c, "Importing")
		}
	}, 400, 400)
	button, _ := window.Find("Background action")
	if err := window.Click("Background field"); err != nil {
		t.Fatal(err)
	}
	loading = true
	window.Frame()
	window.Press(button.X+button.W/2, button.Y+button.H/2)
	window.Release(button.X+button.W/2, button.Y+button.H/2)
	window.Type("blocked")
	window.Key(0, ui.KeyEscape)
	if clicks != 0 || value != "original" {
		t.Fatalf("loading allowed background input: clicks=%d value=%q", clicks, value)
	}
	if _, ok := window.AccessByRole(ui.RoleProgress, "Importing"); !ok {
		t.Fatal("loading spinner should remain visible after Escape")
	}
	loading = false
	window.Frame()
	if err := window.Click("Background action"); err != nil || clicks != 1 {
		t.Fatalf("background action did not recover: %v clicks=%d", err, clicks)
	}
}
