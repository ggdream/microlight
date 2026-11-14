package chat

import (
	"os"
	"path/filepath"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fullAccessRequest struct {
	chat    *conversation
	readyAt time.Time
}

func (app *chatApp) chooseAccess(chat *conversation, mode localtools.Mode, now time.Time) {
	if app.selected() != chat {
		return
	}
	mode = mode.Normalized()
	app.fullAccess = nil
	if mode == localtools.Full {
		app.fullAccess = &fullAccessRequest{chat: chat, readyAt: now.Add(3 * time.Second)}
		return
	}
	app.applyAccess(chat, mode)
}

func (app *chatApp) confirmFullAccess(now time.Time) {
	request := app.fullAccess
	if request == nil || now.Before(request.readyAt) {
		return
	}
	app.fullAccess = nil
	if app.selected() == request.chat {
		app.applyAccess(request.chat, localtools.Full)
	}
}

func (app *chatApp) applyAccess(chat *conversation, mode localtools.Mode) {
	app.accessError = ""
	if chat == nil {
		app.draftAccess = mode
		return
	}
	if chat.accessMode == mode {
		return
	}
	if app.store == nil {
		app.accessError = locales.PermissionSaveFailed()
		return
	}
	result, err := app.store.Exec("UPDATE conversations SET access_mode = ? WHERE id = ?", mode, chat.ID)
	if err != nil {
		app.accessError = locales.PermissionSaveFailed()
		return
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		app.accessError = locales.PermissionSaveFailed()
		return
	}
	chat.accessMode = mode
	if app.active == chat && app.runningAccess != nil {
		app.runningAccess.SetMode(mode)
	}
}

func (app *chatApp) fullAccessDialog(c *ui.Context) {
	request := app.fullAccess
	if request == nil {
		return
	}
	if app.selected() != request.chat {
		app.fullAccess = nil
		return
	}
	title := locales.FullAccessTitle()
	label := locales.ConfirmFullAccess()
	now := c.Now()
	remaining := request.readyAt.Sub(now)
	disabled := remaining > 0
	if disabled {
		seconds := int((remaining + time.Second - 1) / time.Second)
		label = locales.ConfirmFullAccessCountdown(seconds)
		c.After(remaining - time.Duration(seconds-1)*time.Second)
	}
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .38))
		panel.Label(title).Width(384).MaxWidthPercent(90).Padding(20).Gap(12).
			Background(white).Border(1, border).Radius(13).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .22))
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			accessModeIcon(c, localtools.Full)
			ui.Text(c, title).FontSize(16).FontWeight(680).TextColor(ink)
		})
		ui.Text(c, locales.FullAccessBody()).FontSize(13).TextColor(muted)
		ui.Row(c).Justify(ui.End).Gap(8).Margin(5, 0, 0).Children(func() {
			cancel := pointerButton(c, false).Label(locales.Cancel()).AutoFocus().Height(34).PaddingX(13).
				AlignItems(ui.Center).Radius(7).Background(side).Border(1, border)
			if cancel.Hovered() {
				cancel.Background(hover)
			}
			cancel.Children(func() {
				ui.Text(c, locales.Cancel()).FontSize(13).TextColor(ink)
			}).OnClick(func() { app.fullAccess = nil })
			bg, fg := errorFG, white
			if disabled {
				bg, fg = side, muted
			}
			button := pointerButton(c, disabled).Label(label).MinWidth(138).Height(34).PaddingX(13).
				AlignItems(ui.Center).Justify(ui.Center).Radius(7).Background(bg)
			if button.Hovered() && !disabled {
				button.Background(ui.Hex("#833339"))
			}
			button.Children(func() {
				ui.Text(c, label).FontSize(13).FontWeight(620).TextColor(fg)
			}).OnClick(func() {
				if open && !disabled && app.fullAccess == request {
					app.confirmFullAccess(now)
				}
			})
		})
	})
	if !open {
		app.fullAccess = nil
	}
}

func accessModeIcon(c *ui.Context, mode localtools.Mode) {
	switch mode {
	case localtools.Smart:
		icon(c, "shield_checkmark", muted, 15)
	case localtools.Full:
		icon(c, "shield_error", ui.Hex("#B35B13"), 15)
	default:
		icon(c, "hand_left", muted, 15)
	}
}

func (app *chatApp) accessControl(c *ui.Context, chat *conversation) {
	selected := app.draftAccess.Normalized()
	if chat != nil {
		selected = chat.accessMode
	}
	title := locales.AgentPermissions()
	now := c.Now()
	dropdown := ui.SelectBase(c.Key("conversation-access"), &selected)
	trigger := dropdown.Trigger.Label(title).Value(selected.Label()).Tooltip(title).
		Height(30).PaddingX(8).Shrink(0).AlignItems(ui.Center).Gap(5).Radius(6).Cursor(ui.CursorPointer)
	if trigger.Hovered() {
		trigger.Background(side)
	}
	trigger.Children(func() {
		accessModeIcon(c, selected)
		ui.Text(c, selected.Label()).FontSize(11).TextColor(muted)
		ui.Icon(c, icons.SVG("ios_chevron_right")).Size(12, 12).Shrink(0).Rotate(90).TextColor(muted)
	}).OnChange(func() { app.chooseAccess(chat, selected, now) })
	dropdown.Popup(func(panel ui.Element) {
		panel.Width(300).Padding(5).Gap(2).Background(white).Border(1, border).Radius(8)
		for _, choice := range []struct {
			mode localtools.Mode
			hint string
		}{
			{localtools.Ask, locales.ModeAskHint()},
			{localtools.Smart, locales.ModeSmartHint()},
			{localtools.Full, locales.ModeFullHint()},
		} {
			item := dropdown.Item(choice.mode).Label(choice.mode.Label()).Padding(7, 8).Gap(8).Radius(5).
				AlignItems(ui.Start).Cursor(ui.CursorPointer)
			if item.Highlighted() || choice.mode == selected {
				item.Background(tint)
			}
			item.Children(func() {
				accessModeIcon(c, choice.mode)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, choice.mode.Label()).FontSize(12).FontWeight(620).TextColor(ink)
					ui.Text(c, choice.hint).FontSize(11).TextColor(muted)
				})
				if choice.mode == selected {
					icon(c, "check", accent, 13)
				}
			})
		}
		ui.Text(c, locales.ModelDataHint()).Padding(5, 8).FontSize(11).TextColor(muted)
	})
}

func (app *chatApp) workspaceControl(c *ui.Context, chat *conversation) {
	path := app.draftWorkspace
	if chat != nil {
		path = chat.workspace
	}
	target := app.draftTarget
	if chat != nil {
		target = chat.target
	}
	label, tooltip := locales.NoWorkspace(), locales.Workspace()
	if path != "" {
		label, tooltip = filepath.Base(path), path
	} else if chat != nil {
		tooltip += "\n" + locales.WorkspaceLocked()
	}
	if target.Kind != "" {
		label, tooltip = target.Label(), target.Label()+"\n"+target.Directory
	}
	if chat != nil {
		control := ui.Row(c).Gap(5).PaddingX(7).Shrink(0).AlignItems(ui.Center).Tooltip(tooltip)
		if target.Kind != "" {
			control.Cursor(ui.CursorPointer).OnClick(func() { app.openWorkspaceForm(chat) })
		}
		control.Children(func() {
			icon(c, "folder", muted, 15)
			ui.Text(c, label).MaxWidth(100).NoWrap().Ellipsis("…").FontSize(11).TextColor(muted)
		})
		return
	}
	ui.Row(c).AlignItems(ui.Center).Gap(2).Shrink(0).Children(func() {
		theme.PopupMenu(c.Key("workspace-menu"), func(c *ui.Context) ui.Element {
			button := pointerButton(c.Key("conversation-workspace"), app.pickingWorkspace).Label(locales.Workspace()).
				Tooltip(tooltip).Gap(5).AlignItems(ui.Center).Height(30).PaddingX(7).Radius(6).Shrink(0)
			if button.Hovered() {
				button.Background(side)
			}
			return button.Children(func() {
				icon(c, "folder", muted, 15)
				ui.Text(c, label).MaxWidth(100).NoWrap().Ellipsis("…").FontSize(11).TextColor(muted)
			})
		},
			theme.PopupItem{Label: locales.WorkspaceLocal(), Icon: icons.Menu("desktop"), OnSelect: app.pickWorkspace},
			theme.PopupItem{Label: locales.WorkspaceDocker(), Icon: icons.Menu("cube"), OnSelect: func() { app.openNewWorkspaceForm("container") }},
			theme.PopupItem{Label: locales.WorkspaceRemote(), Icon: icons.Menu("desktop_signal"), OnSelect: func() { app.openNewWorkspaceForm("remote") }},
			theme.PopupItem{Label: locales.WorkspaceBrowser(), Icon: icons.Menu("desktop"), OnSelect: func() { app.openNewWorkspaceForm("browser") }},
			theme.PopupItem{Label: locales.WorkspaceDesktop(), Icon: icons.Menu("desktop_signal"), OnSelect: func() { app.openNewWorkspaceForm("desktop") }},
		)
		if path != "" || target.Kind != "" {
			clearLabel := locales.ClearWorkspace()
			clear := pointerButton(c.Key("clear-workspace"), app.pickingWorkspace).Label(clearLabel).Tooltip(clearLabel).
				Size(22, 22).Radius(6).AlignItems(ui.Center).Justify(ui.Center)
			if clear.Hovered() {
				clear.Background(side)
			}
			clear.Children(func() {
				icon(c, "close", muted, 12)
			}).OnClick(func() { app.clearWorkspaceSelection() })
		}
	})
}

func (app *chatApp) pickWorkspace() {
	app.pickingWorkspace = true
	parent, current, draft := app.win, app.draftWorkspace, app.draftVersion
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            parent,
			Title:             locales.ChooseWorkspace(),
			DefaultPath:       current,
			ButtonLabel:       locales.Choose(),
			Directory:         true,
			CreateDirectories: true,
		})
		parent.Update(func() {
			app.pickingWorkspace = false
			if app.selected() != nil || app.draftVersion != draft {
				return
			}
			if err != nil {
				app.accessError = locales.DirectoryPickerFailed()
				return
			}
			if len(paths) == 0 {
				return
			}
			path, err := filepath.EvalSymlinks(paths[0])
			if err != nil || !filepath.IsAbs(path) {
				app.accessError = locales.MissingWorkspace()
				return
			}
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				app.accessError = locales.MissingWorkspace()
				return
			}
			app.clearWorkspaceSelection()
			app.draftWorkspace, app.accessError = path, ""
		})
	}()
}
