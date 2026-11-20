package chat

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAppMessageDialogsReturnChoiceAndKeepNextRequest(t *testing.T) {
	first := &messageDialog{
		options: mygo.MessageOptions{Title: "Import", Message: "Replace saved settings?", Buttons: []string{"Cancel", "Apply settings"}},
		result:  make(chan mygo.MessageResult, 1),
	}
	second := &messageDialog{
		options: mygo.MessageOptions{Title: "Update", Message: "Check complete", Buttons: []string{"Dismiss"}},
		result:  make(chan mygo.MessageResult, 1),
	}
	app := &chatApp{messageDialogs: []*messageDialog{first, second}}
	view := ui.NewTester(app.messageDialog, 600, 500)
	select {
	case <-first.result:
		t.Fatal("confirmation resolved before user choice")
	default:
	}
	if err := view.Click("Apply settings"); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-first.result:
		if result.Button != 1 {
			t.Fatalf("wrong import choice: %d", result.Button)
		}
	default:
		t.Fatal("confirmation did not reach its waiting worker")
	}
	if !view.HasText("Check complete") {
		t.Fatal("next queued message was lost")
	}
	view.Key(0, ui.KeyEscape)
	select {
	case result := <-second.result:
		if result.Button != 0 {
			t.Fatalf("wrong cancel choice: %d", result.Button)
		}
	default:
		t.Fatal("Escape did not dismiss the alert")
	}
	if len(app.messageDialogs) != 0 {
		t.Fatal("dismissed request remained visible")
	}
}
