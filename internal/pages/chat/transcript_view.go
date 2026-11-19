package chat

import (
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

func (app *chatApp) transcript(c *ui.Context, chat *conversation) {
	layout := windowLayout(c)
	chat.list.FollowEnd = true
	interfaces := chat.transcriptInterface()
	elastic.List(c.Key(chat.ID), &chat.list, len(chat.Turns), func(i int, offset elastic.Offset) {
		entry := chat.Turns[i]
		offset.Apply(ui.Column(c).FillWidth().AlignItems(ui.Center)).Children(func() {
			row := ui.Row(c).FillWidth().MaxWidth(conversationWidth).AlignItems(ui.Start).Gap(layout.Gutter(12))
			if entry.Role == "user" {
				if layout.Compact() {
					row.Padding(0, layout.X(24)+layout.Gutter(12))
				}
				row.Justify(ui.End).Children(func() {
					app.userMessage(c.Key(entry), chat, i, entry)
				})
				return
			}
			if layout.Compact() {
				row.Padding(0, layout.X(24)+layout.Gutter(12), 0, 0)
			}
			row.Children(func() {
				size := float32(34)
				if layout.Compact() {
					size = layout.X(24)
				}
				ui.Image(c, app.brandIcon).Size(size, size).Shrink(0).Fit(ui.Contain)
				response := ui.Column(c.Key(entry)).Grow(1).MinWidth(0).Gap(5).Padding(3, 0, 0)
				showActions := touchControls(c) || response.Hovered() || response.FocusWithin()
				showStatistics := !app.desktop.HideStatistics && entry.State != "pending" && entry.State != "waiting" && entry.Metrics.Requests > 0
				elapsed := replyElapsed(entry)
				response.Children(func() {
					hasReasoning, open := replyReasoning(entry)
					replyProgress(c.Key("reply-progress"), entry, hasReasoning, open)
					app.replyBody(c, chat, entry, interfaces, hasReasoning && open)
					app.replyDeliverables(c.Key("artifacts"), entry, chat.workspace)
					if len(entry.Clarifications) > 0 {
						app.clarificationCards(c, chat, entry)
					}
					if entry.Error != "" {
						message := entry.Error
						if message == replyRequestFailed || message == locales.In("zh-Hans").ReplyFailed() {
							message = locales.ReplyFailed()
						}
						ui.Text(c, message).FontSize(viewport.Font(c, 12)).TextColor(errorFG)
						if entry.State == "error" && i == len(chat.Turns)-1 {
							clarificationButton(c.Key("continue-failed"), locales.ContinueFailedTask(), chat.isRunning(), func() { app.continueFailed(chat, entry) })
						}
					}
					if entry.State == "pending" {
						if status := replyRunningStatus(entry); status != "" {
							label := ui.Text(c.Key("running-status"), status).AlignSelf(ui.Start).
								FontSize(layout.Text(12)).FontWeight(500).TextColor(ink).Padding(3, 0)
							flowThinkingText(c, label)
						}
					} else if entry.State != "waiting" {
						app.replyFooter(c.Key("reply-footer"), chat, entry, canRegenerateReply(chat, i), showActions, showStatistics, elapsed)
					}
				})
			})
		})
	}).Grow(1).MinHeight(0).Padding(layout.Y(layout.Density(27, 14)), layout.Gutter(26)).Gap(layout.Y(layout.Density(24, 16)))
}
