package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"os"
	"path/filepath"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/elastic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fullAccessRequest struct {
	chat         *conversation
	acknowledged bool
}

func (app *chatApp) chooseAccess(chat *conversation, mode localtools.Mode) {
	if app.selected() != chat {
		return
	}
	mode = mode.Normalized()
	app.fullAccess = nil
	if mode == localtools.Full {
		app.fullAccess = &fullAccessRequest{chat: chat}
		return
	}
	app.applyAccess(chat, mode)
}

func (app *chatApp) confirmFullAccess() {
	request := app.fullAccess
	if request == nil || !request.acknowledged {
		return
	}
	app.fullAccess = nil
	if app.selected() == request.chat {
		app.applyAccess(request.chat, localtools.Full)
	}
}

func (app *chatApp) applyAccess(chat *conversation, mode localtools.Mode) {
	if app.deferBrowserAction(func() { app.applyAccess(chat, mode) }) {
		return
	}
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
	if chat.isRunning() {
		chat.run.access.SetMode(mode)
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
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		insetDialog(c, backdrop).Background(ui.RGBA(0, 0, 0, .38))
		panel.Label(title).Width(384).MaxWidthPercent(90).MaxHeight(dialogMaxHeight(c)).Padding(windowLayout(c).Density(20, 14)).Gap(windowLayout(c).Density(12, 8)).
			Background(white).Border(1, border).Radius(13).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .22))
		mobileDialog(c, backdrop, panel)
		dialogBody(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				accessModeIcon(c, localtools.Full)
				ui.Text(c, title).FontSize(viewport.Font(c, 16)).FontWeight(680).TextColor(ink)
			})
			ui.Text(c, locales.FullAccessBody()).FontSize(viewport.Font(c, 13)).TextColor(muted)
			ui.Themed(c, theme.PopupTheme(c)).Background(ui.Transparent).Children(func() {
				ui.Checkbox(c, &request.acknowledged, locales.FullAccessAcknowledgement()).
					FillWidth().FontSize(viewport.Font(c, 13)).Cursor(ui.CursorPointer).FocusRingColor(accent)
			})
		})
		disabled := !request.acknowledged
		dialogActions(c).Children(func() {
			cancel := pointerButton(c, false).Label(locales.Cancel()).AutoFocus().Height(34).PaddingX(13).
				AlignItems(ui.Center).Radius(7).Background(side).Border(1, border)
			if cancel.Hovered() {
				cancel.Background(hover)
			}
			cancel.Children(func() {
				ui.Text(c, locales.Cancel()).FontSize(viewport.Font(c, 13)).TextColor(ink)
			}).OnClick(func() { app.fullAccess = nil })
			bg, fg := errorFG, white
			if disabled {
				bg, fg = side, muted
			}
			button := pointerButton(c, disabled).Label(label).Height(34).PaddingX(13).
				AlignItems(ui.Center).Justify(ui.Center).Radius(7).Background(bg)
			if button.Hovered() && !disabled {
				button.Background(ui.Hex("#833339"))
			}
			button.Children(func() {
				ui.Text(c, label).FontSize(viewport.Font(c, 13)).FontWeight(620).TextColor(fg)
			}).OnClick(func() {
				if open && app.fullAccess == request {
					app.confirmFullAccess()
				}
			})
		})
	})
	if !open {
		app.fullAccess = nil
	}
}

func accessModeGlyph(mode localtools.Mode) string {
	switch mode {
	case localtools.Smart:
		return "shield_checkmark"
	case localtools.Full:
		return "shield_error"
	default:
		return "hand_left"
	}
}

func accessModeIcon(c *ui.Context, mode localtools.Mode) {
	color := muted
	if mode == localtools.Full {
		color = ui.Hex("#B35B13")
	}
	icon(c, accessModeGlyph(mode), color, 15)
}

func (app *chatApp) accessControl(c *ui.Context, chat *conversation) {
	selected := app.draftAccess.Normalized()
	if chat != nil {
		selected = chat.accessMode
	}
	title := locales.AgentPermissions()
	layout := app.composerLayout(c)
	if layout.Compact() {
		trigger := pointerButton(c.Key("access-sheet-trigger"), false).Label(title).Value(selected.Label()).Size(36, 36).Center().Radius(7)
		open := ui.Local(trigger, "open", func() bool { return false })
		if trigger.Hovered() {
			trigger.Background(side)
		}
		trigger.Children(func() { accessModeIcon(c, selected) }).OnClick(func() { *open = true })
		if *open {
			selectionSheet(c.Key("access-sheet"), open, title, .38, func() {
				scroll, offset := elastic.Scroll(c.Key("access-options"))
				scroll.Fill().Children(func() {
					offset.Apply(ui.Column(c)).FillWidth().Padding(layout.Y(12), layout.X(18)).Gap(layout.Y(8)).Children(func() {
						ui.Text(c, title).FontSize(layout.Text(17)).FontWeight(620)
						for _, choice := range []struct {
							mode localtools.Mode
							hint string
						}{
							{localtools.Ask, locales.ModeAskHint()}, {localtools.Smart, locales.ModeSmartHint()}, {localtools.Full, locales.ModeFullHint()},
						} {
							button := pointerButton(c.Key(string(choice.mode)), false).Label(choice.mode.Label()).FillWidth().MinHeight(layout.Target(62)).Padding(8).Gap(12).AlignItems(ui.Center).Radius(8)
							if button.Hovered() {
								button.Background(side)
							}
							button.Children(func() {
								accessModeIcon(c, choice.mode)
								fg, hintFG := ink, muted
								if choice.mode == localtools.Full {
									fg = ui.Hex("#96380F")
									hintFG = fg
								}
								ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
									ui.Text(c, choice.mode.Label()).FontSize(layout.Text(14)).FontWeight(620).TextColor(fg)
									ui.Text(c, choice.hint).FontSize(layout.Text(12)).TextColor(hintFG)
								})
								if choice.mode == selected {
									icon(c, "check", muted, 15)
								}
							}).OnClick(func() { *open = false; app.chooseAccess(chat, choice.mode) })
						}
					})
				})
			})
		}
		return
	}
	dropdown := ui.SelectBase(c.Key("conversation-access"), &selected)
	trigger := dropdown.Trigger.Label(title).Value(selected.Label()).Tooltip(locales.ModelDataHint()).
		Height(composerControlHeight(layout, 30)).PaddingX(layout.Density(8, 4)).Shrink(0).AlignItems(ui.Center).Gap(5).Radius(6).Cursor(ui.CursorPointer)
	if layout.Compact() {
		trigger.MinWidth(34).Justify(ui.Center)
	}
	if trigger.Hovered() {
		trigger.Background(side)
	}
	trigger.Children(func() {
		accessModeIcon(c, selected)
		if !app.composerLayout(c).Compact() {
			ui.Text(c, selected.Label()).FontSize(viewport.Font(c, 11)).TextColor(muted)
			ui.Icon(c, icons.SVG("ios_chevron_right")).Size(12, 12).Shrink(0).Rotate(90).TextColor(muted)
		}
	}).OnChange(func() { app.chooseAccess(chat, selected) })
	dropdown.Popup(func(panel ui.Element) {
		panel.Width(windowLayout(c).FitWidth(356, 12)).Padding(5).Gap(2).Background(white).Border(1, border).Radius(10)
		for _, choice := range []struct {
			mode localtools.Mode
			hint string
		}{
			{localtools.Ask, locales.ModeAskHint()},
			{localtools.Smart, locales.ModeSmartHint()},
			{localtools.Full, locales.ModeFullHint()},
		} {
			label := choice.mode.Label()
			item := dropdown.Item(choice.mode).Label(label).Description(choice.hint).
				Padding(8, 9).Gap(10).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			selectedMode := choice.mode == selected
			checkColor := ui.Hex("#575B5E")
			iconColor, titleColor, hintColor := checkColor, ui.Hex("#1B1F22"), ui.Hex("#707477")
			if choice.mode == localtools.Full {
				iconColor = ui.Hex("#AD6C52")
				titleColor = ui.Hex("#96380F")
				hintColor = titleColor
			}
			if item.Hovered() || (item.Highlighted() && !selectedMode) {
				item.Background(ui.Hex("#F5F5F5"))
			}
			item.Children(func() {
				icon(c, accessModeGlyph(choice.mode), iconColor, 17)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, label).FontSize(viewport.Font(c, 13)).FontWeight(620).TextColor(titleColor)
					ui.Text(c, choice.hint).FillWidth().FontSize(viewport.Font(c, 12)).TextColor(hintColor)
				})
				ui.Box(c).Size(15, 15).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					if selectedMode {
						icon(c, "check", checkColor, 14)
					}
				})
			})
		}
	})
}

func (app *chatApp) workspaceControl(c *ui.Context, chat *conversation) {
	if !desktop.Desktop || chat != nil || app.draftWorkspace != "" {
		return
	}
	label := locales.ChooseWorkspace()
	layout := app.composerLayout(c)
	button := pointerButton(c.Key("conversation-workspace"), app.pickingWorkspace).Label(label).
		Tooltip(label).Gap(5).AlignItems(ui.Center).Height(composerControlHeight(layout, 30)).PaddingX(layout.Density(7, 4)).Radius(6).Shrink(0)
	if layout.Compact() {
		button.MinWidth(34).Justify(ui.Center)
	}
	if button.Hovered() {
		button.Background(side)
	}
	button.Children(func() {
		icon(c, "folder", muted, 15)
		if !app.composerLayout(c).Compact() {
			ui.Text(c, label).FontSize(viewport.Font(c, 11)).TextColor(muted)
		}
	}).OnClick(app.pickWorkspace)
}

func (app *chatApp) pickWorkspace() {
	app.pickingWorkspace = true
	parent, draft := app.win, app.draftVersion
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            parent,
			Title:             locales.ChooseWorkspace(),
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
			app.draftWorkspace, app.accessError = path, ""
		})
	}()
}
