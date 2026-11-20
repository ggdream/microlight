package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
)

func TestConnectionTestToastAndCanceledResult(t *testing.T) {
	for _, failed := range []bool{false, true} {
		app := &chatApp{}
		_, cancel := context.WithCancel(t.Context())
		test := &connectionTest{cancel: cancel}
		app.form.test = test
		var err error
		if failed {
			err = errors.New("fixture connection failure")
		}
		app.connectionTestResult(test, "test-model", err)
		cancel()
		if app.form.test != nil || app.connectionToast == nil {
			t.Fatal("completion did not release the spinner and queue a toast")
		}
		want := locales.ConnectionOK()
		if failed {
			want = locales.ConnectionFailedDetail("test-model")
		}
		window := ui.NewTester(app.connectionTestFeedback, 500, 400)
		window.Advance(250 * time.Millisecond)
		if !window.HasText(want) {
			t.Fatal("missing connection result toast")
		}
		window.Advance(5 * time.Second)
		if window.HasText(want) {
			t.Fatal("result toast did not expire")
		}
	}
	app := &chatApp{}
	_, cancel := context.WithCancel(t.Context())
	test := &connectionTest{cancel: cancel}
	app.form.test = test
	app.clearConnectionTest()
	app.connectionTestResult(test, "old-model", errors.New("stale response"))
	if app.connectionToast != nil {
		t.Fatal("a canceled test showed a stale toast")
	}
}
