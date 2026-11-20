//go:build js && wasm

package webnotify

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"syscall/js"
	"time"

	"github.com/gowebapi/webapi/notification"
)

var (
	ErrDenied      = errors.New("browser notification permission is not granted")
	ErrUnavailable = errors.New("browser notifications require a supported secure context")
)

type Options struct{ Title, Body, Tag, Icon string }
type Notification struct {
	options      Options
	click        func()
	value        js.Value
	registration js.Value
	handlers     []js.Func
}

func Supported() bool {
	return js.Global().Get("isSecureContext").Truthy() && js.Global().Get("Notification").Type() == js.TypeFunction
}
func Granted() bool { return Supported() && notification.Permission() == notification.Granted }

// RequestPermission starts immediately, before the user-action callback returns.
// Its waiter may block on another goroutine, never inside a JS event callback.
func RequestPermission() func(context.Context) error {
	done := make(chan struct{})
	var result error
	finish := func(err error) { result = err; close(done) }
	switch {
	case !Supported():
		finish(ErrUnavailable)
	case Granted():
		finish(nil)
	case notification.Permission() == notification.Denied:
		finish(ErrDenied)
	default:
		var promise js.Value
		err := jsCall(func() { promise = notification.RequestPermission(nil).JSValue() })
		if err != nil {
			finish(err)
		} else {
			settle(promise, func(_ js.Value, err error) {
				if err == nil && !Granted() {
					err = ErrDenied
				}
				finish(err)
			})
		}
	}
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return result
		}
	}
}

func New(options Options) *Notification   { return &Notification{options: options} }
func (n *Notification) OnClick(fn func()) { n.click = fn }

var messages sync.Once
var byTag = map[string]*Notification{}

func (n *Notification) ShowContext(ctx context.Context) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !Supported() {
		return ErrUnavailable
	}
	if !Granted() {
		return ErrDenied
	}
	// Each Show represents a new event. Replacing the previous notification for
	// the same conversation (or test button) must alert the user again.
	options := &notification.Options{Body: n.options.Body, Tag: n.options.Tag, Icon: n.options.Icon, Timestamp: int(time.Now().UnixMilli()), Renotify: true}
	if options.Tag == "" {
		options.Tag = "microlight-test"
	}
	byTag[options.Tag] = n
	defer func() {
		if err != nil && byTag[options.Tag] == n {
			delete(byTag, options.Tag)
		}
	}()
	nav := js.Global().Get("navigator")
	sw := nav.Get("serviceWorker")
	if sw.Truthy() {
		messages.Do(func() {
			listener := js.FuncOf(func(_ js.Value, args []js.Value) any {
				data := args[0].Get("data")
				if data.Truthy() && data.Get("type").String() == "microlight-notification-click" {
					if note := byTag[data.Get("tag").String()]; note != nil {
						delete(byTag, data.Get("tag").String())
						if note.click != nil {
							note.click()
						}
					}
				}
				return nil
			})
			sw.Call("addEventListener", "message", listener)
			// One listener lives for this page's lifetime.
		})
		var promise js.Value
		if err := jsCall(func() { promise = sw.Call("register", "/notification-worker.js") }); err != nil {
			return err
		}
		if _, err := await(ctx, promise); err != nil {
			return err
		}
		reg, err := await(ctx, sw.Get("ready"))
		if err != nil {
			return err
		}
		n.registration = reg
		clientID, err := notificationClient(ctx, reg.Get("active"))
		if err != nil {
			return err
		}
		options.Data = js.ValueOf(map[string]any{"clientID": clientID})
		if err := jsCall(func() { promise = reg.Call("showNotification", n.options.Title, options.JSValue()) }); err != nil {
			return err
		}
		_, err = await(ctx, promise)
		return err
	}
	// Desktop browsers without Service Worker support can use page notifications.
	if err := jsCall(func() { n.value = notification.New(n.options.Title, options).JSValue() }); err != nil {
		return err
	}
	done := make(chan error, 1)
	clicked := js.FuncOf(func(js.Value, []js.Value) any {
		js.Global().Call("focus")
		if n.click != nil {
			n.click()
		}
		return nil
	})
	shown := js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case done <- nil:
		default:
		}
		return nil
	})
	failed := js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case done <- ErrUnavailable:
		default:
		}
		return nil
	})
	closed := js.FuncOf(func(js.Value, []js.Value) any {
		for _, name := range []string{"onclick", "onshow", "onerror", "onclose"} {
			n.value.Set(name, js.Null())
		}
		for _, handler := range n.handlers {
			handler.Release()
		}
		n.handlers = nil
		if byTag[options.Tag] == n {
			delete(byTag, options.Tag)
		}
		return nil
	})
	n.handlers = []js.Func{clicked, shown, failed, closed}
	n.value.Set("onclick", clicked)
	n.value.Set("onshow", shown)
	n.value.Set("onerror", failed)
	n.value.Set("onclose", closed)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		n.Close()
		return ctx.Err()
	}
}
func (n *Notification) Close() {
	tag := n.options.Tag
	if tag == "" {
		tag = "microlight-test"
	}
	if byTag[tag] == n {
		delete(byTag, tag)
	}
	if n.value.Truthy() {
		n.value.Call("close")
	}
	if n.registration.Truthy() {
		go func() {
			tag := n.options.Tag
			if tag == "" {
				tag = "microlight-test"
			}
			notes, err := await(context.Background(), n.registration.Call("getNotifications", map[string]any{"tag": tag}))
			if err == nil {
				for i := 0; i < notes.Length(); i++ {
					notes.Index(i).Call("close")
				}
			}
		}()
	}
}

// Associate delivery with the originating tab, even when several tabs share a URL.
func notificationClient(ctx context.Context, worker js.Value) (string, error) {
	channel := js.Global().Get("MessageChannel").New()
	port := channel.Get("port1")
	done := make(chan string, 1)
	handler := js.FuncOf(func(_ js.Value, args []js.Value) any { done <- args[0].Get("data").String(); return nil })
	port.Set("onmessage", handler)
	defer func() { port.Call("close"); handler.Release() }()
	worker.Call("postMessage", map[string]any{"type": "microlight-notification-client"}, []any{channel.Get("port2")})
	select {
	case id := <-done:
		return id, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func jsCall(fn func()) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("browser notification: %v", value)
		}
	}()
	fn()
	return
}
func settle(promise js.Value, callback func(js.Value, error)) {
	var success, failure js.Func
	success = js.FuncOf(func(_ js.Value, args []js.Value) any {
		success.Release()
		failure.Release()
		callback(args[0], nil)
		return nil
	})
	failure = js.FuncOf(func(_ js.Value, args []js.Value) any {
		success.Release()
		failure.Release()
		callback(js.Undefined(), fmt.Errorf("browser notification: %s", args[0].String()))
		return nil
	})
	promise.Call("then", success, failure)
}
func await(ctx context.Context, promise js.Value) (js.Value, error) {
	type result struct {
		value js.Value
		err   error
	}
	done := make(chan result, 1)
	settle(promise, func(value js.Value, err error) { done <- result{value, err} })
	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		return js.Undefined(), ctx.Err()
	}
}
