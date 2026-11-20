package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/ggdream/microlight/pkg/material3"
)

func (app *chatApp) userMessage(c *ui.Context, chat *conversation, index int, entry *turn) {
	bubble := ui.Column(c).MaxWidthPercent(91).AlignItems(ui.End).Gap(windowLayout(c).Density(4, 8))
	if windowLayout(c).Compact() {
		bubble.FillWidth().MinWidth(0).MaxWidthPercent(100)
	}
	showActions := touchControls(c) || bubble.Hovered() || bubble.FocusWithin()
	bubble.Children(func() {
		if len(entry.Attachments) > 0 {
			ui.Row(c).MaxWidthPercent(100).Wrap().Justify(ui.End).Gap(8).Children(func() {
				for _, attachment := range entry.Attachments {
					app.attachmentTile(c, attachment, 76, nil)
				}
			})
		}
		if edit := app.edit; edit != nil && edit.entry == entry && !edit.inComposer {
			app.messageEditor(c, chat, index, edit)
			return
		}
		if entry.Content != "" {
			body := markdown.View(c.Key("message-body"), userMessageMarkdown(entry.Content), app.md).MinWidth(0).MaxWidthPercent(100).
				Padding(windowLayout(c).Density(10, 7), windowLayout(c).Density(14, 10)).Radius(material3.ShapeMedium).Background(ui.Hex("#E8F3FE"))
			app.selectMessageText(c, body, entry.Content)
		}
		opacity := float32(0)
		if showActions {
			opacity = 1
		}
		actions := ui.Row(c).AlignItems(ui.Center).Gap(windowLayout(c).Density(5, 3)).Opacity(opacity)
		if windowLayout(c).Compact() {
			actions.Margin(-windowLayout(c).Y(6), 0, 0)
		}
		actions.Children(func() {
			if showActions {
				messageTimestamp(c, entry.StartedAt)
			}
			ui.Row(c).Gap(windowLayout(c).Density(1, 0)).Children(func() {
				if entry.Content != "" {
					copyMessageIcon(c, entry, locales.CopyMessage(), locales.MessageCopied())
				}
				messageIcon(c, locales.EditMessage(), "edit", !app.canEditMessage(chat, index), func() {
					app.beginMessageEdit(chat, entry, windowLayout(c).Compact())
				})
			})
		})
	})
}

// Preserve explicit user line endings while leaving code blocks and existing
// Markdown hard breaks intact. The stored message and copied source stay original.
func userMessageMarkdown(source string) string {
	doc := parser.Parse(source)
	var breaks []int
	for i := 0; i < doc.Len(); i++ {
		for _, token := range doc.Inlines(i) {
			if token.Kind == parser.SoftBreak {
				breaks = append(breaks, token.Start)
			}
		}
	}
	if len(breaks) == 0 {
		return source
	}
	var result strings.Builder
	start := 0
	for _, position := range breaks {
		result.WriteString(source[start:position])
		result.WriteString("  ")
		start = position
	}
	result.WriteString(source[start:])
	return result.String()
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
	ui.Text(c, local.Format("15:04")).FontSize(viewport.Font(c, 11)).TextColor(muted).Opacity(.76).
		NoWrap().Tooltip(local.Format("2006-01-02 15:04:05"))
}

func (app *chatApp) messageEditor(c *ui.Context, chat *conversation, index int, edit *messageEdit) {
	layout := windowLayout(c)
	material3.CardStyle(ui.Column(c).Width(510).MaxWidthPercent(100).Gap(layout.Y(8)).Padding(layout.Y(10), layout.X(10)), material3.ElevatedCard).
		Background(white).Children(func() {
		editor := edit.input.View(c.Key(edit), locales.EditMessageBody(), "", layout.Y(104), app.md).
			Padding(layout.Y(5), layout.X(5)).AutoFocus()
		if editor.Shortcut(0, ui.KeyEscape) {
			app.edit = nil
		}
		if editor.Shortcut(ui.Cmd, ui.KeyEnter) {
			app.resendMessage(chat, index, edit, edit.input.Markdown())
		}
		ui.Row(c).Wrap().Justify(ui.End).Gap(layout.X(5)).Children(func() {
			clarificationButton(c, locales.Cancel(), false, func() { app.edit = nil })
			disabled := !app.canEditMessage(chat, index) ||
				(strings.TrimSpace(edit.input.Document.Text().String()) == "" && len(edit.entry.Attachments) == 0)
			clarificationButton(c, locales.Resend(), disabled, func() {
				app.resendMessage(chat, index, edit, edit.input.Markdown())
			})
		})
	})
}

const messageControlRadius = 6

func messageIconFrame(control ui.Element) ui.Element {
	return control.Size(25, 25).Radius(messageControlRadius).AlignItems(ui.Center).Justify(ui.Center)
}

func messageTouchWidth(c *ui.Context) float32 {
	return max(26, min(28, windowLayout(c).X(26)))
}

func messageIcon(c *ui.Context, label, glyph string, disabled bool, action func()) {
	button := pointerButton(c, disabled).Label(label).Tooltip(label).Center().Shrink(0)
	if touchControls(c) {
		button.Size(messageTouchWidth(c), 44)
	} else {
		button.Size(25, 25)
	}
	button.Children(func() {
		size := float32(25)
		if touchControls(c) {
			size = messageTouchWidth(c)
		}
		face := ui.Box(c).Size(size, size).Radius(messageControlRadius).Center()
		if button.Hovered() || button.Pressed() {
			face.Background(hover)
		}
		face.Children(func() { icon(c, glyph, muted, 14) })
	}).OnClick(action)
}
