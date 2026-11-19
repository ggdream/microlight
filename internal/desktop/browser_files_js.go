//go:build js && wasm

package desktop

import (
	"errors"
	"fmt"
	"syscall/js"
)

// OpenBrowserFiles must be called from a goroutine following a user action.
// Browser file names are not OS paths; bytes are persisted in the app database.
func OpenBrowserFiles(multiple bool, accept string, maxBytes int) ([]BrowserFile, error) {
	document := js.Global().Get("document")
	input := document.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("multiple", multiple)
	input.Set("accept", accept)
	input.Get("style").Set("display", "none")
	done := make(chan js.Value, 1)
	change := js.FuncOf(func(js.Value, []js.Value) any { done <- input.Get("files"); return nil })
	cancel := js.FuncOf(func(js.Value, []js.Value) any { done <- js.Null(); return nil })
	input.Set("onchange", change)
	input.Set("oncancel", cancel)
	document.Get("body").Call("appendChild", input)
	input.Call("click")
	selected := <-done
	input.Set("onchange", js.Null())
	input.Set("oncancel", js.Null())
	input.Call("remove")
	change.Release()
	cancel.Release()
	if selected.IsNull() {
		return nil, nil
	}
	if selected.Length() > 8 {
		return nil, errors.New("select at most 8 files")
	}
	var files []BrowserFile
	for i := 0; i < selected.Length(); i++ {
		file := selected.Index(i)
		if file.Get("size").Float() > float64(maxBytes) {
			return nil, fmt.Errorf("file exceeds %d bytes", maxBytes)
		}
		buffer, err := browserPromise(file.Call("arrayBuffer"))
		if err != nil {
			return nil, err
		}
		array := js.Global().Get("Uint8Array").New(buffer)
		data := make([]byte, array.Get("byteLength").Int())
		js.CopyBytesToGo(data, array)
		files = append(files, BrowserFile{Name: file.Get("name").String(), Data: data})
	}
	return files, nil
}

func browserPromise(promise js.Value) (js.Value, error) {
	type result struct {
		value js.Value
		err   error
	}
	done := make(chan result, 1)
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any { done <- result{value: args[0]}; return nil })
	bad := js.FuncOf(func(_ js.Value, args []js.Value) any { done <- result{err: errors.New(args[0].String())}; return nil })
	promise.Call("then", ok, bad)
	r := <-done
	ok.Release()
	bad.Release()
	return r.value, r.err
}

func browserBlob(data []byte, mime string) string {
	array := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(array, data)
	blob := js.Global().Get("Blob").New(js.ValueOf([]any{array}), js.ValueOf(map[string]any{"type": mime}))
	return js.Global().Get("URL").Call("createObjectURL", blob).String()
}

func retireBrowserBlob(url string) {
	var cleanup js.Func
	cleanup = js.FuncOf(func(js.Value, []js.Value) any {
		js.Global().Get("URL").Call("revokeObjectURL", url)
		cleanup.Release()
		return nil
	})
	js.Global().Call("setTimeout", cleanup, 60000)
}

func DownloadBrowserFile(name string, data []byte) error {
	url := browserBlob(data, "application/octet-stream")
	link := js.Global().Get("document").Call("createElement", "a")
	link.Set("href", url)
	link.Set("download", name)
	link.Call("click")
	retireBrowserBlob(url)
	return nil
}

func PreviewBrowserFile(data []byte, mime string) error {
	window := js.Global().Call("open", "about:blank", "_blank")
	if !window.Truthy() {
		return errors.New("browser blocked the preview window")
	}
	url := browserBlob(data, mime)
	document := window.Get("document")
	frame := document.Call("createElement", "iframe")
	// Never execute an attached HTML/SVG document with the app origin's access
	// to IndexedDB, credentials, or the opener.
	frame.Call("setAttribute", "sandbox", "")
	frame.Set("src", url)
	frame.Get("style").Set("cssText", "position:fixed;inset:0;width:100%;height:100%;border:0")
	document.Get("body").Call("appendChild", frame)
	window.Set("opener", js.Null())
	retireBrowserBlob(url)
	return nil
}
