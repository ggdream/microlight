package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"path/filepath"
	"slices"

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/localtools"
	previewui "github.com/ggdream/microlight/internal/pages/chat/preview"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

type toolPreview struct {
	entry     *turn
	index     int
	text      *string
	directory *previewui.Directory
	list      ui.ListState
}

type toolPreviewPanel struct {
	visible bool
	round   *turn
	tabs    []*toolPreview
	active  *toolPreview
	reveal  *toolPreview
}

func (panel *toolPreviewPanel) selectTab(tab *toolPreview) {
	panel.active, panel.reveal, panel.round = tab, tab, tab.entry
	panel.visible = true
}

func (panel *toolPreviewPanel) closeTab(tab *toolPreview) {
	index := slices.Index(panel.tabs, tab)
	if index < 0 {
		return
	}
	panel.tabs = slices.Delete(panel.tabs, index, index+1)
	if panel.active == tab {
		if len(panel.tabs) > 0 {
			panel.selectTab(panel.tabs[min(index, len(panel.tabs)-1)])
		} else {
			panel.active, panel.reveal = nil, nil
		}
	}
}

func (app *chatApp) openRoundChanges(entry *turn) {
	app.toolPanel.visible, app.toolPanel.round = true, entry
	app.toolPanel.active, app.toolPanel.reveal = nil, nil
}

func (app *chatApp) openToolPreview(entry *turn, index int) {
	panel := &app.toolPanel
	for _, tab := range panel.tabs {
		if tab.entry == entry && tab.index == index {
			panel.selectTab(tab)
			return
		}
	}
	preview := &toolPreview{entry: entry, index: index}
	panel.tabs = append(panel.tabs, preview)
	panel.selectTab(preview)
	activity := entry.Activity[index]
	var workspace string
	if selected := app.selected(); selected != nil {
		workspace = selected.workspace
	}
	if activity.Tool == localtools.List {
		path := activity.DisplayTarget()
		if path != "" && !filepath.IsAbs(path) && workspace != "" {
			path = filepath.Join(workspace, path)
		}
		preview.directory = previewui.NewDirectory(path, activity.Output)
		return
	}
	if activity.Output != nil || (activity.Diff != nil && len(activity.Diff.Rows) > 0) ||
		activity.Target == "" || app.win == nil {
		return
	}
	if activity.Tool != localtools.Read && activity.Tool != localtools.Write {
		return
	}
	// Older records can open the current file on an explicit click. A stored
	// result, when present, is always preferred to reading the filesystem again.
	ctx := app.appContext()
	go func() {
		tool := activity.Tool
		if tool == localtools.Write {
			tool = localtools.Read
		}
		content, err := localtools.ReadPreview(ctx, workspace, tool, activity.DisplayTarget())
		if err == nil {
			app.win.Update(func() {
				if ctx.Err() == nil {
					preview.text = &content
				}
			})
		}
	}()
}

func previewName(activity timeline.Entry) string {
	switch activity.Tool {
	case localtools.Read, localtools.List, localtools.Write:
		if path := activity.DisplayTarget(); path != "" {
			return filepath.Base(path)
		}
	}
	if target := activity.ShortTarget(); target != "" {
		return target
	}
	return activity.Label("")
}

func previewIcon(activity timeline.Entry) string {
	if activity.Tool == localtools.Read || activity.Tool == localtools.Write {
		return previewui.FileIcon(activity.DisplayTarget())
	}
	return activity.Icon()
}

func (app *chatApp) toolPreviewPane(c *ui.Context, width float32, workspace string) {
	panel := &app.toolPanel
	round, active := panel.round, panel.active
	tree := previewui.BuildTree(roundChangedFiles(round, workspace), workspace)
	ui.Column(c).Width(width).Shrink(0).FillHeight().MinHeight(0).Clip().
		Background(white).BorderWidth(0, 0, 0, 1).BorderColor(border).Children(func() {
		app.toolPreviewTabs(c.Key("tabs"))
		treeContent := func() {
			selected := -1
			if active != nil && active.entry == round {
				selected = active.index
			}
			tree.View(c.Key("changes"), round, icons, selected,
				func(index int) { app.openToolPreview(round, index) },
				func(path string) { app.showPreviewError(desktop.PreviewFile(path)) })
		}
		previewContent := func() {
			activity := active.entry.Activity[active.index]
			view := c.Key("preview").Key(active)
			if active.directory != nil {
				var update func(func())
				if app.win != nil {
					update = app.win.Update
				}
				active.directory.View(view, icons, previewName(activity), update,
					func(path string) { app.showPreviewError(desktop.PreviewFile(path)) })
				return
			}
			previewui.Content(view, &active.list, activity, previewName(activity), active.text)
		}
		switch {
		case active == nil:
			ui.Box(c).FillWidth().Grow(1).MinHeight(0).Children(treeContent)
		case width >= 410 && tree.HasChanges():
			ui.Row(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Clip().Children(previewContent)
				ui.Box(c).Width(min(225, width*.42)).Shrink(0).FillHeight().
					BorderWidth(0, 0, 0, 1).BorderColor(border).Children(treeContent)
			})
		default:
			previewContent()
		}
	})
}

func (app *chatApp) toolPreviewTabs(c *ui.Context) {
	panel := &app.toolPanel
	layout := windowLayout(c)
	pad := layout.Y(4)
	ui.Row(c).FillWidth().Height(max(layout.Y(40), layout.Target(29)+2*pad+1)).Shrink(0).
		AlignItems(ui.Center).Gap(layout.X(4)).Padding(pad, layout.X(6)).
		Background(side).BorderWidth(0, 0, 1, 0).BorderColor(border).Children(func() {
		scroll, offset := elastic.Horizontal(c.Key("tab-strip"))
		scroll.Grow(1).MinWidth(0).FillHeight().Children(func() {
			offset.Apply(ui.Row(c)).Role(ui.RoleTabList).Gap(4).AlignItems(ui.Center).Children(func() {
				changes := pointerButton(c.Key("round"), false).Role(ui.RoleTab).Label(locales.RoundChanges()).
					AccessSelected(panel.active == nil).Height(windowLayout(c).Target(29)).Shrink(0).PaddingX(7).Gap(5).AlignItems(ui.Center).Radius(5)
				if panel.active == nil || changes.Hovered() {
					changes.Background(white)
				}
				changes.Children(func() {
					icon(c, "document_sync", muted, 15)
					ui.Text(c, locales.RoundChanges()).NoWrap().FontSize(viewport.Font(c, 12))
				}).OnClick(func() { panel.active, panel.reveal = nil, nil })
				for _, tab := range panel.tabs {
					app.toolPreviewTab(c.Key(tab), tab)
				}
			})
		})
		label := locales.ClosePreview()
		closePane := messageIconFrame(pointerButton(c.Key("close-pane"), false).Label(label).Tooltip(label)).
			Size(layout.Target(29), layout.Target(29)).Shrink(0)
		if closePane.Hovered() {
			closePane.Background(white)
		}
		closePane.Children(func() { icon(c, "close", muted, 14) }).OnClick(func() { panel.visible = false })
	})
}

func (app *chatApp) toolPreviewTab(c *ui.Context, tab *toolPreview) {
	panel := &app.toolPanel
	activity := tab.entry.Activity[tab.index]
	name := previewName(activity)
	selected := panel.active == tab
	row := ui.Row(c).Height(windowLayout(c).Target(29)).Shrink(0).AlignItems(ui.Center).Padding(0, 3, 0, 5).Radius(5)
	if selected || row.Hovered() {
		row.Background(white)
	}
	if panel.reveal == tab {
		row.ScrollIntoView()
		panel.reveal = nil
	}
	row.Children(func() {
		pointerButton(c.Key("select"), false).Role(ui.RoleTab).Label(name).AccessSelected(selected).
			Tooltip(activity.DisplayTarget()).FillHeight().Gap(5).AlignItems(ui.Center).Children(func() {
			icon(c, previewIcon(activity), muted, 14)
			ui.Text(c, name).MaxWidth(120).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 12))
		}).OnClick(func() { panel.selectTab(tab) })
		messageIcon(c.Key("close-tab"), locales.CloseFileTab(name), "close", false, func() { panel.closeTab(tab) })
	})
}

func (app *chatApp) composerMenu(c *ui.Context, workspace string) {
	unavailable := app.loadingAttachments || len(app.draftAttachments) >= attachment.MaxPerMessage
	theme.PopupMenu(c, func(c *ui.Context) ui.Element {
		add := pointerButton(c, false).Label(locales.AddAttachment()).Tooltip(locales.AddAttachment()).
			Size(34, 34).Radius(17).AlignItems(ui.Center).Justify(ui.Center)
		add.Children(func() { icon(c, "plus", ink, 18) })
		if add.Hovered() {
			add.Background(hover)
		}
		return add
	},
		theme.PopupItem{Label: locales.UploadFile(), Icon: icons.Menu("document_text"), Disabled: unavailable, OnSelect: app.pickAttachments},
		theme.PopupItem{Label: locales.ChooseSkill(), Icon: icons.Menu("skill"), OnSelect: func() { app.openComposerSkills(workspace, true) }},
		theme.PopupItem{Label: locales.PlanMode(), Icon: icons.Menu("update_tasks")},
	)
}
