//go:build js && wasm

package main

import (
	"context"
	"errors"
	"syscall/js"

	"github.com/ggdream/microlight/pkg/webnotify"
)

var testNote *webnotify.Notification
var notificationClicked bool

func init() {
	js.Global().Set("notificationCheck", js.FuncOf(func(_ js.Value, args []js.Value) any {
		op := args[0].String()
		if op == "clicked" {
			return notificationClicked
		}
		var wait func(context.Context) error
		if op == "request" {
			wait = webnotify.RequestPermission()
		}
		executor := js.FuncOf(func(_ js.Value, callbacks []js.Value) any {
			resolve, reject := callbacks[0], callbacks[1]
			go func() {
				var err error
				switch op {
				case "request":
					err = wait(context.Background())
				case "show":
					testNote = webnotify.New(webnotify.Options{Title: "Microlight browser QA", Body: "Actual browser notification", Tag: "notification-qa", Icon: "/product/web/icon.png"})
					testNote.OnClick(func() { notificationClicked = true; testNote.Close() })
					err = testNote.ShowContext(context.Background())
				case "close":
					testNote.Close()
				}
				if errors.Is(err, webnotify.ErrDenied) {
					resolve.Invoke("denied")
				} else if err != nil {
					reject.Invoke(err.Error())
				} else {
					resolve.Invoke("ok")
				}
			}()
			return nil
		})
		promise := js.Global().Get("Promise").New(executor)
		executor.Release()
		return promise
	}))
}
