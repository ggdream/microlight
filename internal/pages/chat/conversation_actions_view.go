package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"math"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
)

type conversationAction struct {
	chat     *conversation
	deleting bool
	info     bool
	title    string
	failed   bool
}

func (app *chatApp) sidebarChat(c *ui.Context, chat *conversation, selected bool) {
	menu := func(m *ui.Menu) { app.conversationMenu(m, chat) }
	row := ui.Row(c).FillWidth().Height(windowLayout(c).Target(34)).PaddingX(4).AlignItems(ui.Center).Radius(7)
	hovered := row.Hovered()
	color := muted
	if selected {
		row.Background(tint)
		color = accent
	} else if hovered || chat.menuOpen {
		row.Background(hover)
	}
	row.Children(func() {
		button := pointerButton(c.Key("select"), false).Label(chat.Title).Tooltip(chat.Title).
			Grow(1).MinWidth(0).FillHeight().Padding(0, 4).Gap(8).AlignItems(ui.Center)
		if !chat.isRunning() && button.LongPressed() {
			chat.menuOpen = true
		}
		button.Children(func() {
			glyph := "chat"
			if chat.pinned {
				glyph = "pin"
			}
			icon(c, glyph, color, 14)
			sidebarTitle(c.Key("title"), chat.Title, color, hovered && !chat.menuOpen)
		}).OnClick(func() { app.selectChat(chat.ID) })

		size := float32(24)
		if windowLayout(c).Compact() {
			size = 36
		}
		if chat.isRunning() {
			chat.menuOpen = false
			ui.Box(c.Key("activity")).Size(size, size).Shrink(0).
				Role(ui.RoleProgress).Label(locales.Thinking()).Draw(drawActivityRing)
		} else if hovered || chat.menuOpen || touchControls(c) || button.FocusVisible() {
			more := pointerButton(c.Key("more"), false).Label(locales.ChatActions()).Size(size, size).
				Shrink(0).Radius(5).AlignItems(ui.Center).Justify(ui.Center)
			if more.Hovered() || chat.menuOpen {
				more.Background(white)
			}
			more.Children(func() { icon(c, "more_horizontal", color, 16) })
			more.PopoverMenuButton(menu, ui.PopoverMenuButtonOptions{
				At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd, Open: &chat.menuOpen,
			})
		} else {
			ui.Box(c.Key("menu-slot")).Size(size, size).Shrink(0)
		}
	}).PopoverMenu(menu)
}

func sidebarTitle(c *ui.Context, title string, color ui.Color, hovered bool) ui.Element {
	const gap = 24
	fontSize := windowLayout(c).Text(12)
	viewport := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Clip().AccessHidden(true)
	width := viewport.LayoutBounds().W
	textWidth, _ := c.MeasureText(0, ui.Span{Text: title, Size: fontSize})
	textWidth = float32(math.Ceil(float64(textWidth)))
	overflow := width > 0 && textWidth > width
	if width == 0 {
		c.After(time.Millisecond)
	}
	if overflow {
		viewport.MaskLinearGradient(ui.LinearGradient{
			Angle: 90, From: ui.RGB(0, 0, 0), To: ui.Transparent,
			Start: 1 - min(18/width, .4),
		})
	}

	started := ui.Local(viewport, "hover-started", func() time.Time { return time.Time{} })
	scrolling := hovered && overflow && !c.Preferences().ReduceMotion
	var offset float32
	if scrolling {
		now := c.Now()
		if started.IsZero() {
			*started = now
		}
		const pause = 350 * time.Millisecond
		if elapsed := now.Sub(*started); elapsed < pause {
			c.After(pause - elapsed)
		} else {
			const speed = 40 // DIPs per second.
			offset = float32(math.Mod((elapsed-pause).Seconds()*speed, float64(textWidth+gap)))
			c.AnimationFrame()
		}
	} else {
		*started = time.Time{}
	}
	var track ui.Element
	viewport.Children(func() {
		track = ui.Row(c).Shrink(0).Gap(gap).AlignItems(ui.Center).Translate(-offset, 0)
		track.Children(func() {
			ui.Text(c, title).Width(textWidth).Shrink(0).SingleLine().FontSize(fontSize).TextColor(color)
			if scrolling {
				ui.Text(c.Key("repeat"), title).Width(textWidth).Shrink(0).SingleLine().FontSize(fontSize).TextColor(color)
			}
		})
	})
	return track
}

func (app *chatApp) conversationMenu(m *ui.Menu, chat *conversation) {
	disabled := app.store == nil
	if m.Item(locales.RenameChat()).Icon(icons.Menu("edit")).Disabled(disabled).Chosen() {
		app.chatAction = &conversationAction{chat: chat, title: chat.Title}
	}
	pinLabel, pinIcon := locales.PinChat(), "pin"
	if chat.pinned {
		pinLabel, pinIcon = locales.UnpinChat(), "pin_off"
	}
	if m.Item(pinLabel).Icon(icons.Menu(pinIcon)).Disabled(disabled).Chosen() {
		app.togglePin(chat)
	}
	m.Separator()
	if m.Item(locales.ArchiveChat()).Icon(icons.Menu("archive")).Disabled(disabled).Chosen() {
		app.setChatArchived(chat, true)
	}
	if m.Item(locales.DeleteChat()).Icon(icons.Menu("delete")).Danger().
		Disabled(disabled || chat.isRunning()).Chosen() {
		app.chatAction = &conversationAction{chat: chat, deleting: true}
	}
	m.Separator()
	if m.Item(locales.ChatInfo()).Icon(icons.Menu("info")).Chosen() {
		app.chatAction = &conversationAction{chat: chat, info: true}
	}
}

func (app *chatApp) conversationActionDialog(c *ui.Context) {
	action := app.chatAction
	if action == nil {
		return
	}
	if action.info {
		app.conversationInfoDialog(c.Key("info"), action.chat)
		return
	}
	title, confirm, confirmColor := locales.RenameChat(), locales.SaveChatTitle(), accent
	role := ui.RoleDialog
	if action.deleting {
		title, confirm, confirmColor = locales.DeleteChatTitle(), locales.DeleteChat(), errorFG
		role = ui.RoleAlertDialog
	}
	ui.Overlay(c, func() {
		backdrop := ui.Box(c.Key("backdrop")).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Center().Background(ui.RGBA(0, 0, 0, .4)).Modal()
		insetDialog(c, backdrop)
		cancel, submit := backdrop.OverlayShortcut(0, ui.KeyEscape), false
		backdrop.Children(func() {
			panel := ui.Column(c).Role(role).Label(title).Width(340).MaxWidthPercent(90).MaxHeight(dialogMaxHeight(c)).Padding(windowLayout(c).Density(20, 14)).
				Gap(windowLayout(c).Density(12, 8)).Radius(10).Background(white).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .3))
			mobileDialog(c, backdrop, panel)
			panel.Children(func() {
				dialogBody(c, func() {
					ui.Text(c, title).FontSize(viewport.Font(c, 16)).FontWeight(680)
					if action.deleting {
						ui.Text(c, action.chat.Title).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13)).FontWeight(610)
						ui.Text(c, locales.DeleteChatBody()).FontSize(viewport.Font(c, 13)).TextColor(muted)
					} else {
						field := settings.Field(c, locales.ChatTitle(), "", &action.title).AutoFocus()
						submit = field.Shortcut(0, ui.KeyEnter)
					}
					if action.failed {
						ui.Text(c, app.historyErr).FontSize(viewport.Font(c, 12)).TextColor(errorFG)
					}
				})
				dialogActions(c).Children(func() {
					back := pointerButton(c.Key("cancel"), false).Label(locales.Cancel()).Height(34).PaddingX(13).
						AlignItems(ui.Center).Radius(7).Background(side).Border(1, border)
					if action.deleting {
						back.AutoFocus()
					}
					if back.Hovered() {
						back.Background(hover)
					}
					back.Children(func() { ui.Text(c, locales.Cancel()).FontSize(viewport.Font(c, 13)) })
					cancel = back.Clicked() || cancel
					save := pointerButton(c.Key("confirm"), !action.deleting && strings.TrimSpace(action.title) == "").
						Label(confirm).Height(34).PaddingX(13).AlignItems(ui.Center).Radius(7).Background(confirmColor)
					if save.Hovered() {
						save.Background(confirmColor.Alpha(.88))
					}
					save.Children(func() { ui.Text(c, confirm).FontSize(viewport.Font(c, 13)).FontWeight(620).TextColor(white) })
					submit = save.Clicked() || submit
				})
			})
		})
		if cancel {
			app.chatAction = nil
		} else if submit {
			saved := false
			if action.deleting {
				saved = app.deleteChat(action.chat)
			} else if strings.TrimSpace(action.title) != "" {
				saved = app.renameChat(action.chat, action.title)
			}
			action.failed = !saved && app.historyErr != ""
			if saved {
				app.chatAction = nil
			}
		}
	})
}

func (app *chatApp) archiveSettings(c *ui.Context) {
	var archived []settings.ArchivedChat
	for _, chat := range app.history.Chats {
		if chat.archived {
			archived = append(archived, settings.ArchivedChat{ID: chat.ID, Title: chat.Title})
		}
	}
	settings.ArchivedChats(c, icons, archived, app.historyErr, func(id string) {
		for _, chat := range app.history.Chats {
			if chat.ID == id {
				app.setChatArchived(chat, false)
				return
			}
		}
	})
}
