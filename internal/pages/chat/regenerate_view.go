package chat

import (
	"slices"
	"time"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func canRegenerateReply(chat *conversation, index int) bool {
	if index <= 0 || index >= len(chat.Turns) || chat.Turns[index-1].Role != "user" {
		return false
	}
	reply := chat.Turns[index]
	return reply.Role == "assistant" && reply.State != "pending" && reply.State != "waiting"
}

func (app *chatApp) regenerateDialog(c *ui.Context, chat *conversation) {
	reply := app.retryReply
	if reply == nil {
		return
	}
	if chat == nil || chat.isRunning() || chat.waitingReply() != nil || app.backend == nil || app.backend.configErr != nil {
		app.retryReply = nil
		return
	}
	index := slices.Index(chat.Turns, reply)
	if !canRegenerateReply(chat, index) {
		app.retryReply = nil
		return
	}
	historical := index < len(chat.Turns)-1
	open, confirmed := regenerateConfirmation(c, historical)
	if open {
		return
	}
	app.retryReply = nil
	if !confirmed {
		return
	}
	if historical {
		app.regenerateInNewChat(chat, index)
	} else {
		app.forkReply(chat, index, chat.Turns[:index], time.Now())
	}
}

// Historical regeneration leaves the original conversation available in the sidebar.
func (app *chatApp) regenerateInNewChat(original *conversation, index int) {
	branch := &conversation{
		ID: newID(), Title: locales.BranchTitle(original.Title),
		accessMode: original.accessMode, workspace: original.workspace,
		Turns: make([]*turn, index),
	}
	for i, entry := range original.Turns[:index] {
		cloned := *entry
		cloned.Interface = slices.Clone(entry.Interface)
		branch.Turns[i] = &cloned
	}
	app.history.Chats = append([]*conversation{branch}, app.history.Chats...)
	app.selectChat(branch.ID)
	if app.selected() != branch {
		app.history.Chats = app.history.Chats[1:]
		return
	}
	app.generate(branch, index)
}

func regenerateConfirmation(c *ui.Context, hasLaterMessages bool) (open, confirmed bool) {
	open = true
	title, body := locales.RetryTitle(), locales.RetryBody()
	if hasLaterMessages {
		body = locales.RetryBranchBody()
	}
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
