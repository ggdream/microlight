package chat

import (
	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func (app *chatApp) transcript(c *ui.Context, chat *conversation) {
	chat.list.FollowEnd = true
	ui.List(c.Key(chat.ID), &chat.list, len(chat.Turns), func(i int) {
		entry := chat.Turns[i]
		ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
			row := ui.Row(c).FillWidth().MaxWidth(conversationWidth).AlignItems(ui.Start).Gap(12)
			if entry.Role == "user" {
				row.Justify(ui.End).Children(func() {
					app.userMessage(c.Key(entry), chat, i, entry)
				})
				return
			}
			row.Children(func() {
				ui.Box(c).Width(28).Height(28).Radius(8).Background(accent).Draw(drawSpark(white))
				response := ui.Column(c.Key(entry)).Grow(1).MinWidth(0).Gap(5).Padding(3, 0, 0)
				showActions := response.Hovered() || response.FocusWithin()
				response.Children(func() {
					hasReasoning, open := replyReasoning(entry)
					replyProgress(c.Key("reply-progress"), entry, hasReasoning, open)
					app.replyBody(c, entry, hasReasoning && open)
					if len(entry.Clarifications) > 0 {
						app.clarificationCards(c, chat, entry)
					}
					if entry.Error != "" {
						message := entry.Error
						if message == replyRequestFailed || message == locales.In("zh-Hans").ReplyFailed() {
							message = locales.ReplyFailed()
						}
						ui.Text(c, message).FontSize(12).TextColor(errorFG)
					}
					if entry.State == "pending" {
						if status := replyRunningStatus(entry); status != "" {
							label := ui.Text(c.Key("running-status"), status).AlignSelf(ui.Start).
								FontSize(12).TextColor(muted).Padding(3, 0)
							shimmerReplyText(c, label)
						}
					} else if entry.State != "waiting" {
						actions := ui.Row(c).Gap(4).AlignItems(ui.Center)
						if !showActions {
							actions.Opacity(0)
						}
						if !entry.FinishedAt.IsZero() {
							actions.MinHeight(25)
						}
						actions.Children(func() {
							if entry.Content != "" {
								copyMessageIcon(c, entry, locales.Copy(), locales.Copied())
							}
							if i == len(chat.Turns)-1 {
								messageIcon(c, locales.Retry(), "retry", app.active != nil, func() { app.retryReply = entry })
							}
							if showActions {
								messageTimestamp(c, entry.FinishedAt)
							}
						})
					}
				})
			})
		})
	}).Grow(1).MinHeight(0).Padding(27, 26).Gap(24)
}
