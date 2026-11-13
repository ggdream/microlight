package chat

import (
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (app *chatApp) userMessage(c *ui.Context, chat *conversation, index int, entry *turn) {
	bubble := ui.Column(c).MaxWidthPercent(91).AlignItems(ui.End).Gap(4)
	showActions := bubble.Hovered() || bubble.FocusWithin()
	bubble.Children(func() {
		if len(entry.Attachments) > 0 {
			ui.Row(c).Wrap().Justify(ui.End).Gap(6).Children(func() {
				for _, attachment := range entry.Attachments {
					attachmentTile(c, attachment, 76, nil)
				}
			})
		}
		if edit := app.edit; edit != nil && edit.entry == entry {
			app.messageEditor(c, chat, index, edit)
			return
		}
		if entry.Content != "" {
			markdown.View(c.Key("message-body"), entry.Content, app.md).
				Padding(10, 14).Radius(15).Background(tint)
		}
		opacity := float32(0)
		if showActions {
			opacity = 1
		}
		ui.Row(c).AlignItems(ui.Center).Gap(5).Opacity(opacity).Children(func() {
			if showActions {
				messageTimestamp(c, entry.StartedAt)
			}
			ui.Row(c).Gap(1).Children(func() {
				if entry.Content != "" {
					copyMessageIcon(c, entry, locales.CopyMessage(), locales.MessageCopied())
				}
				messageIcon(c, locales.EditMessage(), "edit", !app.canEditMessage(chat, index), func() {
					app.edit = &messageEdit{entry: entry}
					app.edit.input.SetText(entry.Content)
				})
			})
		})
	})
}

func copyMessageIcon(c *ui.Context, entry *turn, label, copiedLabel string) {
	glyph := "copy"
	if remaining := entry.copiedUntil.Sub(c.Now()); remaining > 0 {
		label, glyph = copiedLabel, "check"
		c.After(remaining)
	}
	messageIcon(c, label, glyph, false, func() {
		c.WriteClipboard(entry.Content)
		entry.copiedUntil = c.Now().Add(1500 * time.Millisecond)
	})
}

func messageTimestamp(c *ui.Context, timestamp time.Time) {
	if timestamp.IsZero() {
		return
	}
	local := timestamp.Local()
	ui.Text(c, local.Format("15:04")).FontSize(11).TextColor(muted).Opacity(.76).
		NoWrap().Tooltip(local.Format("2006-01-02 15:04:05"))
}

func (app *chatApp) messageEditor(c *ui.Context, chat *conversation, index int, edit *messageEdit) {
	ui.Column(c).Width(510).MaxWidthPercent(100).Gap(8).Padding(10).Radius(12).
		Border(1, border).Background(white).Children(func() {
		editor := edit.input.View(c.Key(edit), locales.EditMessageBody(), "", 104, app.md).Padding(5)
		if editor.Shortcut(0, ui.KeyEscape) {
			app.edit = nil
		}
		if editor.Shortcut(ui.Cmd, ui.KeyEnter) {
			app.resendMessage(chat, index, edit, edit.input.Markdown())
		}
		ui.Row(c).Justify(ui.End).Gap(5).Children(func() {
			clarificationButton(c, locales.Cancel(), false, func() { app.edit = nil })
			disabled := !app.canEditMessage(chat, index) ||
				(strings.TrimSpace(edit.input.Document.Text().String()) == "" && len(edit.entry.Attachments) == 0)
			clarificationButton(c, locales.Resend(), disabled, func() {
				app.resendMessage(chat, index, edit, edit.input.Markdown())
			})
		})
	})
}

func messageIcon(c *ui.Context, label, glyph string, disabled bool, action func()) {
	button := pointerButton(c, disabled).Label(label).Tooltip(label).Size(25, 25).
		Radius(6).AlignItems(ui.Center).Justify(ui.Center)
	if button.Hovered() {
		button.Background(side)
	}
	button.Children(func() { icon(c, glyph, muted, 14) }).OnClick(action)
}
