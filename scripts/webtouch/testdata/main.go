//go:build js && wasm

package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"syscall/js"
)

func main() {
	var scroll ui.ScrollState
	js.Global().Set("touchScrollY", js.FuncOf(func(js.Value, []js.Value) any { return scroll.Y }))
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Width: 390, Height: 640, Content: ui.View(func(c *ui.Context) {
			ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
				ui.Column(c).Height(3000).FillWidth().Children(func() { ui.Text(c, "Touch scrolling regression") })
			})
		})})
	})
	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
}
