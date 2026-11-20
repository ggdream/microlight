//go:build !js

package desktop

import (
	"context"

	"github.com/egoist/mygo"
)

func NotificationsSupported() bool { return mygo.NotificationsSupported() }
func NewNotification(options mygo.NotificationOptions) *mygo.Notification {
	return mygo.NewNotification(options)
}
func RequestNotificationPermission() func(context.Context) error {
	return func(context.Context) error { return nil }
}
