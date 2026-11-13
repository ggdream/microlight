package chat

import (
	"testing"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"

	"github.com/egoist/mygo/ui"
)

func TestFullAccessDialogCountdown(t *testing.T) {
	app := &chatApp{draftAccess: localtools.Ask}
	surface := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() { app.accessControl(c, nil) })
		app.fullAccessDialog(c)
	}, 600, 400)
	click := func(label string) {
		t.Helper()
		if err := surface.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	open := func() {
		t.Helper()
		click(locales.ModeAsk())
		click(locales.ModeFull())
		if _, shown := surface.Find(locales.FullAccessTitle()); !shown {
			t.Fatal("choosing full access did not open the dialog")
		}
	}
	open()
	click(locales.ConfirmFullAccessCountdown(3))
	if app.draftAccess != localtools.Ask || app.fullAccess == nil {
		t.Fatal("the countdown button must not grant full access")
	}
	for seconds := 2; seconds >= 1; seconds-- {
		surface.Advance(time.Second)
		if _, shown := surface.Find(locales.ConfirmFullAccessCountdown(seconds)); !shown {
			t.Fatalf("the dialog did not show %d seconds remaining", seconds)
		}
	}
	surface.Advance(time.Second)
	click(locales.Cancel())
	if app.draftAccess != localtools.Ask || app.fullAccess != nil {
		t.Fatal("cancelling must leave permissions unchanged")
	}
	open()
	surface.Advance(3 * time.Second)
	if app.draftAccess != localtools.Ask {
		t.Fatal("the countdown must not grant access on its own")
	}
	click(locales.ConfirmFullAccess())
	if app.draftAccess != localtools.Full || app.fullAccess != nil {
		t.Fatal("confirmation after the countdown must grant full access and dismiss the dialog")
	}
}
