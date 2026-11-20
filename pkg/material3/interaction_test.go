package material3

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSwitchKeepsPointerAndKeyboardInteraction(t *testing.T) {
	on := false
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(Monochrome.Theme())
		Switch(c, &on, "Feature")
	}, 200, 100)
	if err := tester.Click("Feature"); err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("pointer did not enable feature")
	}
	tester.Key(0, ui.KeySpace)
	if on {
		t.Fatal("keyboard did not disable feature")
	}
}

func TestButtonVariantsDoNotActivateWhileDisabled(t *testing.T) {
	for _, variant := range []ButtonVariant{ElevatedButton, FilledButton, TonalButton, TextButton} {
		t.Run(string(rune('0'+variant)), func(t *testing.T) {
			calls := 0
			disabled := true
			tester := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(Monochrome.Theme())
				Button(c, "Save", variant, disabled).Width(100).Children(func() {
					ui.Text(c, "Save")
				}).OnClick(func() { calls++ })
			}, 200, 100)
			_ = tester.Click("Save")
			if calls != 0 {
				t.Fatal("disabled action was executed")
			}
			disabled = false
			tester.Key(0, ui.KeyTab)
			if err := tester.Click("Save"); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal("enabled action was not executed once")
			}
		})
	}
}
