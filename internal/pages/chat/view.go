package chat

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/viewport"
)

const (
	conversationWidth   = 724
	replyControlHeight  = 34
	replyControlRadius  = 9
	defaultSidebarWidth = 226
	minSidebarWidth     = 40
	sidebarCollapseAt   = 128
)

func (app *chatApp) view(c *ui.Context) {
	// Inherited typography also covers Markdown and standard MyGo widgets.
	windowTheme := *material3.Monochrome.Theme()
	windowTheme.Font = c.Theme().Font
	windowTheme.Background = canvas
	windowTheme.FontSize = viewport.Font(c, windowTheme.FontSize)
	c.SetTheme(&windowTheme)
	if app.md != nil {
		app.md.Theme = material3.MarkdownTheme(&windowTheme)
		if windowLayout(c).Compact() {
			app.md.Theme.LineHeight = 1.4
		}
	}
	shortcut := "Ctrl K"
	if runtime.GOOS == "darwin" {
		shortcut = "⌘ K"
	} else if desktop.Mobile {
		shortcut = ""
	}
	if !app.shortcutRecording && app.transferLoading == "" {
		c.OnWindowShortcut(ui.Cmd, ui.KeyK, func() { app.selectChat("") })
	}
	selected := app.selected()
	app.syncBackgroundTasks(c)
	if app.sidebarWidth == 0 {
		app.sidebarWidth = defaultSidebarWidth
	}
	layout := windowLayout(c)
	width := layout.Width
	sidebarLimit := min(float32(440), max(float32(defaultSidebarWidth), width-360))
	app.sidebarWidth = min(app.sidebarWidth, sidebarLimit)
	root := ui.Box(c).Fill().Background(canvas).TextColor(ink).Children(func() {
		if !layout.Compact() {
			app.compactHistory = false
		}
		if app.settingsOpen {
			app.settingsPage(c.Key("settings"))
			return
		}
		sidebarWidth := float32(0)
		if !layout.Compact() {
			sidebarWidth = app.sidebarDivider(c, sidebarLimit)
		}
		ui.Row(c).Fill().Children(func() {
			if !layout.Compact() {
				ui.Box(c.Key("desktop-sidebar")).Width(sidebarWidth).FillHeight().Shrink(0).Clip().Children(func() {
					app.sidebar(c, selected, shortcut, sidebarWidth)
				})
				ui.Box(c.Key("sidebar-gap")).Width(1).Shrink(0)
			}
			ui.Column(c.Key("chat-content")).FillHeight().Grow(1).MinWidth(0).AlignItems(ui.Stretch).Clip().Children(func() {
				app.topbar(c, selected)
				if selected == nil {
					app.welcome(c)
				} else {
					ui.Box(c.Key("transcript-region")).Grow(1).MinHeight(0).Children(func() {
						app.transcript(c, selected)
						if layout.Compact() && len(selected.Turns) > 0 && !selected.list.AtEnd() {
							button := material3.FloatingAction(pointerButton(c.Key("latest"), false).Label(locales.BackToLatest()).Absolute().Right(layout.X(16)).Bottom(layout.Y(12)).Size(46, 46).Center())
							if button.Hovered() {
								button.Background(hover)
							}
							button.Children(func() { icon(c, "down", accent, 20) }).OnClick(selected.list.ScrollToEnd)
						}
					})
				}
				app.composer(c, selected)
			})
			if app.toolPanel.visible && layout.Expanded() {
				contentWidth := width - sidebarWidth
				previewWidth := previewPaneWidth(contentWidth)
				var workspace string
				if selected != nil {
					workspace = selected.workspace
				}
				app.toolPreviewPane(c.Key("tool-preview"), previewWidth, workspace)
			}
		})
		if !layout.Expanded() && app.toolPanel.visible {
			layer := ui.Box(c.Key("compact-layer")).Absolute().Top(0).Bottom(0).Left(0).Right(0).
				Background(side).Modal()
			if layer.OverlayShortcut(0, ui.KeyEscape) {
				app.toolPanel.visible = false
			}
			layer.Children(func() {
				var workspace string
				if selected != nil {
					workspace = selected.workspace
				}
				app.toolPreviewPane(c.Key("tool-preview"), width, workspace)
			})
		} else if layout.Compact() && app.compactHistory {
			app.historyDrawer(c.Key("history-drawer"), width, selected)
		}
	})
	app.linkPreview(c, root)
	app.renderImagePreview(c.Key("image-preview"))
	app.regenerateDialog(c.Key("regenerate-dialog"), selected)
	app.fullAccessDialog(c.Key("full-access-dialog"))
	app.conversationActionDialog(c.Key("conversation-action"))
	app.messageDialog(c.Key("message-dialog"))
	app.settingsTransferFeedback(c.Key("transfer-feedback"))
	app.persistDraft()
}
