package chat

import (
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
)

var conversationMenuTheme = func() *ui.Theme {
	t := ui.LightTheme()
	t.Background, t.Border, t.Text, t.TextMuted = side, border, ink, muted
	t.Accent, t.AccentText, t.Danger = tint, ink, errorFG
	return t
}()

type conversationAction struct {
	chat     *conversation
	deleting bool
	title    string
	failed   bool
}

func (app *chatApp) sidebarChat(c *ui.Context, chat *conversation, selected bool) {
	menu := func(m *ui.Menu) { app.conversationMenu(m, chat) }
	row := ui.Row(c).FillWidth().Height(34).PaddingX(4).AlignItems(ui.Center).Radius(7)
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
		button.Children(func() {
			glyph := "chat"
			if chat.pinned {
				glyph = "pin"
			}
			icon(c, glyph, color, 14)
			ui.Text(c, chat.Title).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12).TextColor(color)
		}).OnClick(func() { app.selectChat(chat.ID) })

		indicator := ui.Box(c.Key("activity")).Size(14, 14).Margin(0, 2).Shrink(0)
		if app.active == chat {
			indicator.Role(ui.RoleProgress).Label(locales.Thinking()).Draw(drawActivityRing)
		}
		if hovered || chat.menuOpen || button.FocusVisible() {
			more := pointerButton(c.Key("more"), false).Label(locales.ChatActions()).Size(24, 24).
				Shrink(0).Radius(5).AlignItems(ui.Center).Justify(ui.Center)
			if more.Hovered() || chat.menuOpen {
				more.Background(white)
			}
			more.Children(func() { icon(c, "more_horizontal", color, 16) })
			more.PopoverMenuButton(menu, ui.PopoverMenuButtonOptions{
				At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd, Open: &chat.menuOpen,
			})
		} else {
			ui.Box(c.Key("menu-slot")).Size(24, 24).Shrink(0)
		}
	}).PopoverMenu(menu)
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
		Disabled(disabled || app.active == chat).Chosen() {
		app.chatAction = &conversationAction{chat: chat, deleting: true}
	}
}

func (app *chatApp) conversationActionDialog(c *ui.Context) {
	action := app.chatAction
	if action == nil {
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
		cancel, submit := backdrop.OverlayShortcut(0, ui.KeyEscape), false
		backdrop.Children(func() {
			ui.Column(c).Role(role).Label(title).Width(340).MaxWidthPercent(90).Padding(20).
				Gap(12).Radius(10).Background(white).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .3)).Children(func() {
				ui.Text(c, title).FontSize(16).FontWeight(680)
				if action.deleting {
					ui.Text(c, action.chat.Title).SingleLine().Ellipsis("…").FontSize(13).FontWeight(610)
					ui.Text(c, locales.DeleteChatBody()).FontSize(13).TextColor(muted)
				} else {
					field := settings.Field(c, locales.ChatTitle(), "", &action.title).AutoFocus()
					submit = field.Shortcut(0, ui.KeyEnter)
				}
				if action.failed {
					ui.Text(c, app.historyErr).FontSize(12).TextColor(errorFG)
				}
				ui.Row(c).Justify(ui.End).Gap(8).Margin(4, 0, 0).Children(func() {
					back := pointerButton(c.Key("cancel"), false).Label(locales.Cancel()).Height(34).PaddingX(13).
						AlignItems(ui.Center).Radius(7).Background(side).Border(1, border)
					if action.deleting {
						back.AutoFocus()
					}
					if back.Hovered() {
						back.Background(hover)
					}
					back.Children(func() { ui.Text(c, locales.Cancel()).FontSize(13) })
					cancel = back.Clicked() || cancel
					save := pointerButton(c.Key("confirm"), !action.deleting && strings.TrimSpace(action.title) == "").
						Label(confirm).Height(34).PaddingX(13).AlignItems(ui.Center).Radius(7).Background(confirmColor)
					if save.Hovered() {
						save.Background(confirmColor.Alpha(.88))
					}
					save.Children(func() { ui.Text(c, confirm).FontSize(13).FontWeight(620).TextColor(white) })
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
