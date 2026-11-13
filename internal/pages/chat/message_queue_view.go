package chat

import (
	"strings"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

func (app *chatApp) busySendSettings(c *ui.Context) {
	title := locales.BusySendSetting()
	labels := map[busySendMode]string{
		busySendQueue: locales.QueueMessage(),
		busySendSteer: locales.DirectSteer(),
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).
		Background(white).Radius(13).Border(1, border).Padding(12, 16).Children(func() {
		ui.Text(c, title).Grow(1).MinWidth(0).FontSize(15).FontWeight(660)
		selected := app.busySend
		if selected != busySendSteer {
			selected = busySendQueue
		}
		dropdown := ui.SelectBase(c.Key("busy-send-select"), &selected)
		trigger := dropdown.Trigger.Label(title).Value(labels[selected]).Width(148).Height(34).Shrink(0).
			PaddingX(10).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).
			Border(1, border).Radius(7).Background(canvas).Cursor(ui.CursorPointer).FocusRingColor(accent)
		if trigger.Hovered() {
			trigger.Background(side)
		}
		trigger.Children(func() {
			ui.Text(c, labels[selected]).FontSize(12).TextColor(ink)
			ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Shrink(0).Rotate(90).TextColor(muted)
		}).OnChange(func() { app.chooseBusySendMode(selected) })
		dropdown.Popup(func(panel ui.Element) {
			panel.Padding(4).Gap(2).Background(white).Border(1, border).Radius(8)
			for _, option := range []busySendMode{busySendQueue, busySendSteer} {
				item := dropdown.Item(option).Label(labels[option]).Padding(6, 8).Radius(5).
					AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Cursor(ui.CursorPointer)
				if item.Highlighted() {
					item.Background(tint)
				}
				item.Children(func() {
					ui.Text(c, labels[option]).FontSize(12).TextColor(ink)
					if option == selected {
						icon(c, "check", accent, 14)
					}
				})
			}
		})
	})
}

func (app *chatApp) messageQueue(c *ui.Context, chat *conversation) {
	if chat == nil || len(chat.queue) == 0 {
		return
	}
	ui.Column(c).FillWidth().MaxWidth(conversationWidth).Gap(2).Padding(4, 8).
		Background(white).Radius(13).Border(1, border).Children(func() {
		scroll, offset := elastic.Scroll(c)
		scroll.FillWidth().MaxHeight(168).Children(func() {
			offset.Apply(ui.Column(c)).FillWidth().Children(func() {
				for _, entry := range chat.queue {
					if edit := app.queueEdit; edit != nil && edit.id == entry.ID {
						app.queueMessageEditor(c.Key(entry.ID), chat, entry, edit)
					} else {
						app.queueMessageRow(c.Key(entry.ID), chat, entry)
					}
				}
			})
		})
		if app.active == nil && chat.waitingReply() == nil {
			disabled := app.queueEdit != nil && app.queueEdit.id == chat.queue[0].ID
			label := locales.ResumeQueue()
			button := pointerButton(c.Key("resume-queue"), disabled).Label(label).AlignSelf(ui.End).
				Padding(4, 7).Gap(5).Radius(6).AlignItems(ui.Center)
			color := accent
			if disabled {
				color = muted
			} else if button.Hovered() {
				button.Background(tint)
			}
			button.Children(func() {
				icon(c, "queue_continue", color, 15)
				ui.Text(c, label).FontSize(12).TextColor(color)
			}).OnClick(func() { app.startNextQueued(chat) })
		}
	})
}

func (app *chatApp) queueMessageRow(c *ui.Context, chat *conversation, entry queuedMessage) {
	label := strings.Join(strings.Fields(entry.Text), " ")
	if label == "" {
		var names []string
		for _, file := range entry.Attachments {
			names = append(names, file.Name)
		}
		label = strings.Join(names, ", ")
	}
	ui.Row(c).FillWidth().Height(32).Gap(8).PaddingX(4).AlignItems(ui.Center).Children(func() {
		icon(c, "queue_item", muted, 15)
		ui.Text(c, label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Clip().
			FontSize(13).TextColor(ink).Tooltip(label)
		if app.active == chat {
			steerLabel := locales.Steer()
			steer := pointerButton(c.Key("steer"), false).Label(steerLabel).
				Tooltip(steerLabel).Shrink(0).Gap(4).Padding(3, 5).Radius(6).AlignItems(ui.Center)
			color := muted
			if steer.Hovered() {
				steer.Background(tint)
				color = accent
			}
			steer.Children(func() {
				icon(c, "queue_steer", color, 15)
				ui.Text(c, locales.QueueSteerAction()).FontSize(12).TextColor(color)
			}).OnClick(func() { app.startQueuedMessage(chat, entry.ID, true) })
		}
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
			messageIcon(c.Key("remove"), locales.RemoveQueuedMessage(), "delete", false, func() {
				app.updateQueuedMessage(chat, entry.ID, nil)
			})
			ui.Themed(c.Key("actions"), conversationMenuTheme).Background(ui.Transparent).Children(func() {
				label := locales.QueuedMessageActions()
				button := pointerButton(c, false).Label(label).Tooltip(label).Size(25, 25).
					Radius(6).AlignItems(ui.Center).Justify(ui.Center)
				if button.Hovered() {
					button.Background(side)
				}
				button.Children(func() { icon(c, "more_horizontal", muted, 15) })
				button.PopoverMenuButton(func(menu *ui.Menu) {
					if menu.Item(locales.EditMessage()).Icon(icons.Menu("edit")).Chosen() {
						app.queueEdit = &queuedMessageEdit{id: entry.ID}
						app.queueEdit.input.SetText(entry.Text)
					}
				}, ui.PopoverMenuButtonOptions{At: ui.AnchorTopEnd, Self: ui.AnchorBottomEnd})
			})
		})
	})
}

func (app *chatApp) queueMessageEditor(c *ui.Context, chat *conversation, entry queuedMessage, edit *queuedMessageEdit) {
	ui.Column(c).FillWidth().Gap(4).Padding(4, 6).Children(func() {
		editor := edit.input.View(c.Key("text"), locales.EditMessageBody(), "", 52, app.md).Padding(4).
			Background(canvas).Border(1, border).Radius(6)
		if editor.Shortcut(0, ui.KeyEscape) {
			app.queueEdit = nil
		}
		save := func() {
			text := edit.input.Markdown()
			app.updateQueuedMessage(chat, entry.ID, &text)
		}
		if editor.Shortcut(ui.Cmd, ui.KeyEnter) {
			save()
		}
		ui.Row(c).Justify(ui.End).Gap(5).Children(func() {
			clarificationButton(c, locales.Cancel(), false, func() { app.queueEdit = nil })
			disabled := strings.TrimSpace(edit.input.Document.Text().String()) == "" && len(entry.Attachments) == 0
			clarificationButton(c, locales.SaveQueuedMessage(), disabled, save)
		})
	})
}
