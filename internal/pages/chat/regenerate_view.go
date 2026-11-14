package chat

import (
	"time"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func (app *chatApp) regenerateDialog(c *ui.Context, chat *conversation) {
	reply := app.retryReply
	if reply == nil {
		return
	}
	open, confirmed := regenerateConfirmation(c)
	if open {
		return
	}
	app.retryReply = nil
	if !confirmed || chat == nil || len(chat.Turns) < 2 || app.active != nil || app.backend.configErr != nil {
		return
	}
	index := len(chat.Turns) - 1
	if chat.Turns[index] == reply && !reply.hasUnresolvedRemoteCommands() {
		app.forkReply(chat, index, chat.Turns[:index], time.Now())
	}
}

func regenerateConfirmation(c *ui.Context) (open, confirmed bool) {
	open = true
	title, body := locales.RetryTitle(), locales.RetryBody()
	ui.Overlay(c, func() {
		backdrop := ui.Box(c.Key("regenerate-backdrop")).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Center().Background(ui.RGBA(0, 0, 0, .4)).Modal()
		cancelKey := backdrop.OverlayShortcut(0, ui.KeyEscape)
		confirmKey := backdrop.OverlayShortcut(0, ui.KeyEnter)
		var cancelClick, confirmClick bool
		backdrop.Children(func() {
			ui.Column(c).Role(ui.RoleAlertDialog).Label(title).Description(body).
				Width(300).MaxWidthPercent(90).Padding(20).Gap(12).Radius(10).Background(white).
				Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .3)).Children(func() {
				ui.Text(c, title).FontSize(16).FontWeight(680).TextColor(ink)
				ui.Text(c, body).FontSize(13).TextColor(muted)
				ui.Row(c).Wrap().Justify(ui.End).Gap(8).Margin(5, 0, 0).Children(func() {
					cancel := pointerButton(c, false).Label(locales.Cancel()).Height(34).PaddingX(13).
						AlignItems(ui.Center).Radius(7).Background(side).Border(1, border)
					if cancel.Hovered() {
						cancel.Background(hover)
					}
					cancel.Children(func() {
						ui.Text(c, locales.Cancel()).FontSize(13).TextColor(ink)
					})
					confirm := pointerButton(c, false).Label(locales.ConfirmRetry()).AutoFocus().Height(34).PaddingX(13).
						AlignItems(ui.Center).Radius(7).Background(accent)
					if confirm.Hovered() {
						confirm.Background(ui.Hex("#125943"))
					}
					confirm.Children(func() {
						ui.Text(c, locales.ConfirmRetry()).FontSize(13).FontWeight(620).TextColor(white)
					})
					// Return remains the default action even after Tab focuses Cancel.
					confirmKey = cancel.Shortcut(0, ui.KeyEnter) || confirmKey
					cancelClick, confirmClick = cancel.Clicked(), confirm.Clicked()
				})
			})
		})
		switch {
		case cancelKey:
			open = false
		case confirmKey || confirmClick:
			open, confirmed = false, true
		case cancelClick:
			open = false
		}
	})
	return
}
