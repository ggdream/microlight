package chat

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
	previewui "github.com/ggdream/microlight/internal/pages/chat/preview"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

type toolPreview struct {
	entry *turn
	index int
	text  *string
	list  ui.ListState
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
	if activity.Output != nil || (activity.Diff != nil && len(activity.Diff.Rows) > 0) ||
		activity.Target == "" || app.win == nil {
		return
	}
	if activity.Tool != localtools.Read && activity.Tool != localtools.List && activity.Tool != localtools.Write {
		return
	}
	var workspace string
	if selected := app.selected(); selected != nil {
		workspace = selected.workspace
	}
	// Older records can open the current file on an explicit click. A stored
	// result, when present, is always preferred to reading the filesystem again.
	go func() {
		tool := activity.Tool
		if tool == localtools.Write {
			tool = localtools.Read
		}
		content, err := localtools.ReadPreview(context.Background(), workspace, tool, activity.DisplayTarget())
		if err == nil {
			app.win.Update(func() { preview.text = &content })
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
		switch strings.ToLower(filepath.Ext(activity.DisplayTarget())) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
			return "image"
		case ".go", ".js", ".ts", ".jsx", ".tsx", ".css", ".html", ".json", ".py", ".rs", ".sh":
			return "code"
		default:
			return "document_text"
		}
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
			tree.View(c.Key("changes"), round, icons, selected, func(index int) { app.openToolPreview(round, index) })
		}
		previewContent := func() {
			activity := active.entry.Activity[active.index]
			previewui.Content(c.Key("preview").Key(active), &active.list, activity, previewName(activity), active.text)
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
		case tree.HasChanges():
			ui.Box(c).FillWidth().Height(145).Shrink(0).
				BorderWidth(0, 0, 1, 0).BorderColor(border).Children(treeContent)
			previewContent()
		default:
			previewContent()
		}
	})
}

func (app *chatApp) toolPreviewTabs(c *ui.Context) {
	panel := &app.toolPanel
	ui.Row(c).FillWidth().Height(40).Shrink(0).AlignItems(ui.Center).Gap(4).Padding(4, 6).
		Background(side).BorderWidth(0, 0, 1, 0).BorderColor(border).Children(func() {
		scroll, offset := elastic.Horizontal(c.Key("tab-strip"))
		scroll.Grow(1).MinWidth(0).FillHeight().Children(func() {
			offset.Apply(ui.Row(c)).Role(ui.RoleTabList).Gap(4).AlignItems(ui.Center).Children(func() {
				changes := pointerButton(c.Key("round"), false).Role(ui.RoleTab).Label(locales.RoundChanges()).
					AccessSelected(panel.active == nil).Height(29).Shrink(0).PaddingX(7).Gap(5).AlignItems(ui.Center).Radius(5)
				if panel.active == nil || changes.Hovered() {
					changes.Background(white)
				}
				changes.Children(func() {
					icon(c, "document_sync", muted, 15)
					ui.Text(c, locales.RoundChanges()).NoWrap().FontSize(12)
				}).OnClick(func() { panel.active, panel.reveal = nil, nil })
				for _, tab := range panel.tabs {
					app.toolPreviewTab(c.Key(tab), tab)
				}
			})
		})
		messageIcon(c.Key("close-pane"), locales.ClosePreview(), "close", false, func() { panel.visible = false })
	})
}

func (app *chatApp) toolPreviewTab(c *ui.Context, tab *toolPreview) {
	panel := &app.toolPanel
	activity := tab.entry.Activity[tab.index]
	name := previewName(activity)
	selected := panel.active == tab
	row := ui.Row(c).Height(29).Shrink(0).AlignItems(ui.Center).Padding(0, 3, 0, 5).Radius(5)
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
			ui.Text(c, name).MaxWidth(120).SingleLine().Ellipsis("…").FontSize(12)
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
