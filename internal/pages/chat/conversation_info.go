package chat

import (
	"encoding/binary"
	"math"
	"time"
	"uuid"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/elastic"
)

func conversationTimes(chat *conversation) (created, last time.Time, knownCreation bool) {
	id, err := uuid.Parse(chat.ID)
	if err == nil && id[6]>>4 == 7 && id[8]&0xc0 == 0x80 {
		// UUIDv7 begins with an unsigned 48-bit Unix timestamp in milliseconds.
		created = time.UnixMilli(int64(binary.BigEndian.Uint64(id[:8]) >> 16))
		knownCreation = true
	}
	for _, message := range chat.Turns {
		for _, at := range [2]time.Time{message.StartedAt, message.FinishedAt} {
			if at.IsZero() {
				continue
			}
			if !knownCreation && (created.IsZero() || at.Before(created)) {
				created = at
			}
			if at.After(last) {
				last = at
			}
		}
	}
	return created, last, knownCreation
}

func (app *chatApp) conversationInfoDialog(c *ui.Context, chat *conversation) {
	created, latest, knownCreation := conversationTimes(chat)
	locale := i18n.Locale()
	format := func(at time.Time) string {
		if at.IsZero() {
			return locales.ChatTimeUnavailable()
		}
		return i18n.FormatDate(locale, at.Local(), i18n.DateOptions{
			Date: i18n.MediumStyle, Time: i18n.MediumStyle, HourCycle: c.Region().HourCycle,
		})
	}
	createdLabel := locales.ChatCreated()
	if !knownCreation && !created.IsZero() {
		createdLabel = locales.ChatFirstRecorded()
	}
	workspace := chat.workspace
	if workspace == "" {
		workspace = locales.WorkspaceUnspecified()
	}
	fields := []struct{ label, value string }{
		{locales.ChatTitle(), chat.Title},
		{createdLabel, format(created)},
		{locales.ChatLastActivity(), format(latest)},
		{locales.ChatMessageCount(), i18n.FormatNumber(locale, len(chat.Turns), i18n.NumberOptions{})},
		{locales.Workspace(), workspace},
		{locales.AgentPermissions(), chat.accessMode.Label()},
	}

	title, closeLabel := locales.ChatInfo(), locales.CloseChatInfo()
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .38))
		panel.Label(title).Width(450).MaxWidthPercent(90).Padding(20).Gap(14).
			Background(white).Border(1, border).Radius(13).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .22))
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			icon(c, "info", accent, 18)
			ui.Text(c, title).FontSize(16).FontWeight(680)
		})
		ui.Column(c).FillWidth().PaddingX(12).Radius(8).Background(canvas).Border(1, border).Children(func() {
			for index, field := range fields {
				row := ui.Row(c.Key(index)).FillWidth().AlignItems(ui.Start).Padding(8, 0).Gap(12)
				if index != 0 {
					row.BorderWidth(1, 0, 0, 0).BorderColor(border)
				}
				row.Children(func() {
					ui.Text(c, field.label).Width(125).Shrink(0).FontSize(12).TextColor(muted)
					if index == 0 {
						conversationInfoTitle(c.Key("title"), field.value)
						return
					}
					ui.Text(c, field.value).Grow(1).MinWidth(0).FontSize(12).Selectable()
				})
			}
		})
		button := pointerButton(c.Key("close"), false).Label(closeLabel).AlignSelf(ui.End).
			AutoFocus().Height(34).PaddingX(14).AlignItems(ui.Center).
			Radius(7).Background(side).Border(1, border)
		if button.Hovered() {
			button.Background(hover)
		}
		button.Children(func() { ui.Text(c, closeLabel).FontSize(13).FontWeight(610) }).
			OnClick(func() { app.chatAction = nil })
	})
	if !open {
		app.chatAction = nil
	}
}

func conversationInfoTitle(c *ui.Context, title string) {
	row := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Start).Gap(8)
	expanded := ui.Local(row, "expanded", func() bool { return false })
	width := row.LayoutBounds().W
	// LayoutBounds is available after the dialog's first frame.
	if width == 0 {
		c.After(time.Millisecond)
	}
	const fontSize = 12
	textWidth, _ := c.MeasureText(0, ui.Span{Text: title, Size: fontSize})
	overflow := width > 0 && math.Ceil(float64(textWidth)) > float64(width)
	if !overflow {
		*expanded = false
	}
	row.Children(func() {
		if *expanded {
			scroll, offset := elastic.Scroll(c.Key("full-title"))
			scroll.Grow(1).MinWidth(0).MaxHeight(124).Children(func() {
				offset.Apply(ui.Text(c, title)).FillWidth().FontSize(fontSize).Selectable()
			})
		} else {
			ui.Text(c, title).Grow(1).MinWidth(0).FontSize(fontSize).SingleLine().Ellipsis("…")
		}
		if !overflow {
			return
		}
		label := locales.ChatTitleView()
		if *expanded {
			label = locales.ChatTitleCollapse()
		}
		button := pointerButton(c.Key("toggle"), false).Label(label).Expanded(*expanded).
			Shrink(0).Radius(4)
		if button.Hovered() {
			button.Background(hover)
		}
		button.Children(func() { ui.Text(c, label).FontSize(fontSize).TextColor(accent) }).
			OnClick(func() { *expanded = !*expanded })
	})
}
