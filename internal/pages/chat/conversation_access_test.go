package chat

import (
	"testing"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"

	"github.com/egoist/mygo/ui"
)

func TestFullAccessDialogRequiresAcknowledgement(t *testing.T) {
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
	click(locales.ConfirmFullAccess())
	if app.draftAccess != localtools.Ask || app.fullAccess == nil {
		t.Fatal("the unchecked dialog must not grant full access")
	}
	click(locales.FullAccessAcknowledgement())
	click(locales.FullAccessAcknowledgement())
	click(locales.ConfirmFullAccess())
	if app.draftAccess != localtools.Ask || app.fullAccess == nil {
		t.Fatal("clearing the checkbox must disable confirmation again")
	}
	click(locales.FullAccessAcknowledgement())
	click(locales.Cancel())
	if app.draftAccess != localtools.Ask || app.fullAccess != nil {
		t.Fatal("cancelling must leave permissions unchanged")
	}
	open()
	if app.fullAccess.acknowledged {
		t.Fatal("reopening must clear the checkbox")
	}
	click(locales.FullAccessAcknowledgement())
	if app.draftAccess != localtools.Ask {
		t.Fatal("checking the box must not grant access on its own")
	}
	click(locales.ConfirmFullAccess())
	if app.draftAccess != localtools.Full || app.fullAccess != nil {
		t.Fatal("confirming after checking the box must grant full access and dismiss the dialog")
	}
}
