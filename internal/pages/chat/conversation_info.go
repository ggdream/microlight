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
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/viewport"
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

	layout := windowLayout(c)
	fontSize := layout.Text(13)

	title, closeLabel := locales.ChatInfo(), locales.CloseChatInfo()
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		insetDialog(c, backdrop).Background(ui.RGBA(0, 0, 0, .38))
		panel.Label(title).Width(460).MaxWidthPercent(90).MaxHeight(dialogMaxHeight(c)).
			Padding(layout.Y(24), layout.X(24)).Gap(layout.Y(16))
		mobileDialog(c, backdrop, panel)
		panel.Background(material3.Monochrome.SurfaceBright)
		dialogBody(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				icon(c, "info", accent, 18)
				ui.Text(c, title).FontSize(viewport.Font(c, 16)).FontWeight(680)
			})
			ui.Column(c).FillWidth().Gap(layout.Y(16)).Padding(layout.Y(8), 0, 0, 0).Children(func() {
				material3.DetailField(c.Key("name"), fields[0].label, func() {
					ui.Row(c).FillWidth().Children(func() {
						conversationInfoTitle(c.Key("title"), fields[0].value)
					})
				})
				material3.Divider(c, material3.DividerOptions{Space: 1})
				field := func(index int) {
					material3.DetailField(c.Key(index), fields[index].label, func() {
						ui.Text(c, fields[index].value).FillWidth().MinWidth(0).FontSize(fontSize).TextColor(ink).Selectable()
					})
				}
				pair := func(first, second int) {
					if layout.Compact() {
						field(first)
						field(second)
						return
					}
					ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(layout.X(24)).Children(func() {
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { field(first) })
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { field(second) })
					})
				}
				pair(1, 2)
				pair(3, 5)
				material3.Divider(c, material3.DividerOptions{Space: 1})
				field(4)
			})
		})
		buttonHeight := layout.Target(34)
		if layout.Compact() {
			buttonHeight = max(30, layout.Y(30))
		}
		dialogActions(c).Children(func() {
			button := material3.Button(c.Key("close"), closeLabel, material3.ElevatedButton, false).
				AutoFocus().Height(buttonHeight).PaddingX(layout.X(14)).FocusRingColor(accent)
			button.Children(func() { ui.Text(c, closeLabel).FontSize(viewport.Font(c, 13)).FontWeight(610) }).
				OnClick(func() { app.chatAction = nil })
		})
	})
	if !open {
		app.chatAction = nil
	}
}

func conversationInfoTitle(c *ui.Context, title string) {
	layout := windowLayout(c)
	row := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(layout.X(8))
	expanded := ui.Local(row, "expanded", func() bool { return false })
	width := row.LayoutBounds().W
	// LayoutBounds is available after the dialog's first frame.
	if width == 0 {
		c.After(time.Millisecond)
	}
	fontSize := layout.Text(14)
	textWidth, _ := c.MeasureText(0, ui.Span{Text: title, Size: fontSize})
	overflow := width > 0 && math.Ceil(float64(textWidth)) > float64(width)
	if !overflow {
		*expanded = false
	}
	row.Children(func() {
		if *expanded {
			scroll, offset := elastic.Scroll(c.Key("full-title"))
			scroll.Grow(1).MinWidth(0).MaxHeight(layout.Y(124)).Children(func() {
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
		button.AlignItems(ui.Center)
		if button.Hovered() {
			button.Background(hover)
		}
		button.Children(func() { ui.Text(c, label).FontSize(fontSize).TextColor(accent) }).
			OnClick(func() { *expanded = !*expanded })
	})
}
