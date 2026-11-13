package chat

import (
	"errors"
	"log"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo"
)

func (app *chatApp) notifyReply(chatID string, reply *turn) {
	var title, body string
	switch reply.State {
	case "done":
		title, body = locales.ReplyCompleted(), locales.ViewReply()
	case "error":
		title, body = locales.ReplyNotificationFailed(), locales.ViewError()
	case "waiting":
		title, body = locales.InputNeeded(), locales.AnswerQuestion()
		if len(reply.Clarifications) > 0 && reply.Clarifications[len(reply.Clarifications)-1].Approval != nil {
			title, body = locales.ApprovalNeeded(), locales.ViewApproval()
		}
	default:
		return
	}

	if app.win.IsDestroyed() || app.chatInForeground() {
		return
	}
	app.unreadReplies++
	mygo.App.SetBadgeCount(app.unreadReplies)
	if !mygo.NotificationsSupported() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		Title: title,
		Body:  body,
		Group: chatID,
	})
	n.OnClick(func() {
		n.Close()
		app.win.Update(func() {
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
		})
	})
	go func() {
		if app.chatInForeground() {
			return
		}
		if err := n.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
			log.Printf("发送回复通知失败：%v", err)
		}
	}()
}

func (app *chatApp) chatInForeground() bool {
	return mygo.App.IsActive() && app.win.IsFocused()
}
