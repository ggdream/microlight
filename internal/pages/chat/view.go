package chat

import (
	"runtime"

	"github.com/egoist/mygo/ui"
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
	shortcut := "Ctrl K"
	if runtime.GOOS == "darwin" {
		shortcut = "⌘ K"
	}
	c.OnWindowShortcut(ui.Cmd, ui.KeyK, func() { app.selectChat("") })
	selected := app.selected()
	if app.sidebarWidth == 0 {
		app.sidebarWidth = defaultSidebarWidth
	}
	width, _ := c.Size()
	sidebarLimit := min(float32(440), max(float32(defaultSidebarWidth), width-360))
	app.sidebarWidth = min(app.sidebarWidth, sidebarLimit)
	root := ui.Box(c).Fill().Background(canvas).TextColor(ink).Children(func() {
		if app.settingsOpen {
			app.settingsPage(c.Key("settings"))
			return
		}
		sidebarWidth := app.sidebarDivider(c, sidebarLimit)
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Width(sidebarWidth).FillHeight().Shrink(0).Clip().Children(func() {
				app.sidebar(c, selected, shortcut, sidebarWidth)
			})
			ui.Box(c).Width(1).Shrink(0)
			ui.Column(c).FillHeight().Grow(1).MinWidth(0).AlignItems(ui.Stretch).Clip().Children(func() {
				app.topbar(c, selected)
				if selected == nil {
					app.welcome(c)
				} else {
					app.transcript(c, selected)
				}
				app.composer(c, selected)
			})
			if app.toolPanel.visible {
				contentWidth := width - sidebarWidth
				previewWidth := min(680, contentWidth*.47, max(180, contentWidth-300))
				var workspace string
				if selected != nil {
					workspace = selected.workspace
				}
				app.toolPreviewPane(c.Key("tool-preview"), previewWidth, workspace)
			}
		})
	})
	app.linkPreview(c, root)
	app.regenerateDialog(c.Key("regenerate-dialog"), selected)
	app.fullAccessDialog(c.Key("full-access-dialog"))
	app.conversationActionDialog(c.Key("conversation-action"))
	app.persistDraft()
}
