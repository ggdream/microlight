//go:build js && wasm

package desktop

import (
	"context"
	"errors"

	"github.com/egoist/mygo"
	"github.com/ggdream/microlight/pkg/webnotify"
)

func NotificationsSupported() bool { return webnotify.Supported() }

type browserNotification struct{ *webnotify.Notification }

func NewNotification(options mygo.NotificationOptions) *browserNotification {
	return &browserNotification{webnotify.New(webnotify.Options{Title: options.Title, Body: options.Body, Tag: options.Group, Icon: "/icon.png"})}
}
func mapNotificationError(err error) error {
	if errors.Is(err, webnotify.ErrDenied) {
		return mygo.ErrNotificationsDenied
	}
	return err
}
func (n *browserNotification) ShowContext(ctx context.Context) error {
	return mapNotificationError(n.Notification.ShowContext(ctx))
}
func RequestNotificationPermission() func(context.Context) error {
	wait := webnotify.RequestPermission()
	return func(ctx context.Context) error { return mapNotificationError(wait(ctx)) }
}
