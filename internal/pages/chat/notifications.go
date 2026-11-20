package chat

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"

	"github.com/egoist/mygo"
)

func replyNotification(reply *turn) (title, body string) {
	switch reply.State {
	case "done":
		title, body = locales.ReplyCompleted(), reply.Content
		if strings.TrimSpace(body) == "" {
			body = locales.ViewReply()
		}
	case "error":
		title, body = locales.ReplyNotificationFailed(), reply.Error
		if strings.TrimSpace(body) == "" {
			body = locales.ViewError()
		}
	case "waiting":
		title, body = locales.InputNeeded(), locales.AnswerQuestion()
		if len(reply.Clarifications) > 0 {
			q := reply.Clarifications[len(reply.Clarifications)-1]
			body = q.displayTitle()
			if q.Approval != nil {
				title = locales.ApprovalNeeded()
				body += " · " + q.Approval.Target
			}
		}
	}
	return title, notificationText(body)
}

func notificationText(body string) string {
	// The Windows notification backend has a 256 UTF-16 unit buffer.
	const limit = 240
	body = strings.Join(strings.Fields(strings.ReplaceAll(body, "\x00", "")), " ")
	units := 0
	for i, char := range body {
		units++
		if char > 0xffff {
			units++
		}
		if units >= limit {
			return strings.TrimSpace(body[:i]) + "…"
		}
	}
	return body
}

func (app *chatApp) sendTestNotification() {
	if app.testingNotification || app.win == nil || app.win.IsDestroyed() {
		return
	}
	if !desktop.NotificationsSupported() {
		app.notificationTestStatus = settingsStatus{message: locales.TestNotificationUnavailable(), failed: true}
		return
	}
	app.testingNotification = true
	app.notificationTestStatus = settingsStatus{}
	ctx, win := app.appContext(), app.win
	n := desktop.NewNotification(mygo.NotificationOptions{
		Title: locales.TestNotificationTitle(),
		Body:  locales.TestNotificationBody(),
	})
	n.OnClick(n.Close)
	waitPermission := desktop.RequestNotificationPermission()
	go func() {
		err := waitPermission(ctx)
		if err == nil {
			err = n.ShowContext(ctx)
		}
		win.Update(func() {
			app.testingNotification = false
			switch {
			case err == nil:
				app.notificationTestStatus = settingsStatus{message: locales.TestNotificationSent()}
			case errors.Is(err, mygo.ErrNotificationsDenied):
				app.notificationTestStatus = settingsStatus{message: notificationDeniedMessage(), failed: true}
			case !errors.Is(err, context.Canceled):
				slog.Error("send test notification", "error", err)
				app.notificationTestStatus = settingsStatus{message: locales.TestNotificationFailed(), failed: true}
			}
		})
	}()
}

func (app *chatApp) notifyReply(chatID string, reply *turn) {
	title, body := replyNotification(reply)
	if title == "" || app.win == nil || app.win.IsDestroyed() || app.chatInForeground() {
		return
	}
	app.unreadReplies++
	mygo.App.SetBadgeCount(app.unreadReplies)
	mode := clampNotificationMode(app.desktop.Notification)
	if mode == notifyOff || mode == notifyAction && reply.State == "done" || !desktop.NotificationsSupported() {
		return
	}
	n := desktop.NewNotification(mygo.NotificationOptions{
		Title: title,
		Body:  body,
		Group: chatID,
	})
	n.OnClick(func() {
		n.Close()
		app.win.Update(func() {
			if app.win.IsDestroyed() {
				return
			}
			if app.history.Selected != chatID {
				app.selectChat(chatID)
			} else if app.settingsOpen && !app.deferSettingsLeave(app.closeSettings) {
				app.closeSettings()
			}
			if app.win.IsMinimized() {
				app.win.Restore()
			}
			mygo.App.Show()
			app.win.Show()
			mygo.App.Focus()
			if chat := app.selected(); chat != nil && chat.ID == chatID {
				chat.list.ScrollToEnd()
			}
		})
	})
	ctx := app.appContext()
	go func() {
		if ctx.Err() != nil || app.chatInForeground() {
			return
		}
		if err := n.ShowContext(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, mygo.ErrNotificationsDenied) {
			slog.Error("发送回复通知失败", "error", err)
		}
	}()
}

func (app *chatApp) chatInForeground() bool {
	return mygo.App.IsActive() && app.win.IsFocused()
}

func notificationDeniedMessage() string {
	if desktop.Web {
		return locales.BrowserNotificationDenied()
	}
	return locales.TestNotificationDenied()
}

// Begin while still handling the user's selection; never prompt from a reply.
func (app *chatApp) requestBrowserNotificationPermission() {
	if !desktop.Web {
		return
	}
	wait := desktop.RequestNotificationPermission()
	go func() {
		err := wait(app.appContext())
		app.win.Update(func() {
			if app.win.IsDestroyed() {
				return
			}
			if err == nil {
				app.notificationTestStatus = settingsStatus{}
			} else if errors.Is(err, mygo.ErrNotificationsDenied) {
				app.notificationTestStatus = settingsStatus{message: locales.BrowserNotificationDenied(), failed: true}
			} else if err != nil && !errors.Is(err, context.Canceled) {
				app.notificationTestStatus = settingsStatus{message: locales.TestNotificationUnavailable(), failed: true}
			}
		})
	}()
}
