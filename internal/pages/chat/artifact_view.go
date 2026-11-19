package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/chat/preview"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/diff"
)

func (entry *turn) conflictPaths() []string {
	var paths []string
	for _, item := range entry.Activity {
		if item.Conflict && !slices.Contains(paths, item.ResolvedTarget) {
			paths = append(paths, item.ResolvedTarget)
		}
	}
	return paths
}

func (entry *turn) deliverables() []int {
	var indexes []int
	seen := make(map[string]bool)
	for i := len(entry.Activity) - 1; i >= 0; i-- {
		item := entry.Activity[i]
		if (item.Tool != localtools.Write && item.Tool != localtools.PublishArtifact) ||
			item.Conflict || item.ResolvedTarget == "" || item.Output == nil || seen[item.ResolvedTarget] {
			continue
		}
		indexes = append(indexes, i)
		seen[item.ResolvedTarget] = true
	}
	slices.Reverse(indexes)
	return indexes
}

func (app *chatApp) replyDeliverables(c *ui.Context, entry *turn, workspace string) {
	for _, path := range entry.conflictPaths() {
		ui.Row(c.Key(path)).FillWidth().AlignItems(ui.Start).Gap(8).Padding(8, 10).
			Radius(7).Background(errorFG.Alpha(.07)).Children(func() {
			icon(c, "shield_error", errorFG, 16)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, locales.FileConflict()).FontSize(viewport.Font(c, 12)).FontWeight(620).TextColor(errorFG)
				ui.Text(c, path).FontSize(viewport.Font(c, 12)).TextColor(muted).Selectable().FillWidth()
				ui.Text(c, locales.FileConflictDetail()).FontSize(viewport.Font(c, 12)).TextColor(muted).FillWidth()
			})
		})
	}
	changes := roundChangedFiles(entry, workspace)
	files := slices.Clone(changes)
	for _, index := range entry.deliverables() {
		path := entry.Activity[index].ResolvedTarget
		if !slices.ContainsFunc(changes, func(change preview.Change) bool { return change.Path == path }) {
			files = append(files, preview.Change{Path: path, Index: index})
		}
	}
	images := entry.deliverableImages()
	if len(files) == 0 && len(images) == 0 {
		return
	}
	card := ui.Column(c.Key("deliverables")).FillWidth().Margin(6, 0, 0).Radius(11).
		Background(white).Border(1, border)
	expanded := ui.Local(card, "expanded", func() bool { return false })
	card.Children(func() {
		app.deliverablesHeader(c.Key("summary"), entry, changes)
		ui.Column(c.Key("items")).FillWidth().Padding(3, 5, 5).
			BorderWidth(1, 0, 0, 0).BorderColor(border).Children(func() {
			const initial = 3
			visible := len(files)
			if !*expanded {
				visible = min(initial, visible)
			}
			for _, file := range files[:visible] {
				app.fileDeliverableRow(c.Key(file.Path), entry, file, workspace)
			}
			if remaining := len(files) - initial; remaining > 0 {
				label := locales.ArtifactShowMore(remaining)
				if *expanded {
					label = locales.ArtifactShowLess()
				}
				toggle := pointerButton(c.Key("expand"), false).Label(label).Expanded(*expanded).
					AlignSelf(ui.Start).AlignItems(ui.Center).Padding(6, 8).Gap(5).Radius(6)
				if toggle.Hovered() {
					toggle.Background(side)
				}
				toggle.Children(func() {
					ui.Text(c, label).FontSize(viewport.Font(c, 13)).FontWeight(610)
					angle := float32(90)
					if *expanded {
						angle = -90
					}
					ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Rotate(angle).TextColor(muted)
				}).OnClick(func() { *expanded = !*expanded })
			}
			for _, image := range images {
				app.imageDeliverableRow(c.Key(image.source), image)
			}
		})
	})
}

func (app *chatApp) deliverablesHeader(c *ui.Context, entry *turn, changes []preview.Change) {
	layout := windowLayout(c)
	label, glyph := locales.Deliverables(), "folder_open"
	var totals diff.Lines
	hasCounts := false
	if len(changes) > 0 {
		label, glyph = locales.ArtifactFilesEdited(len(changes)), "document_sync"
		for _, change := range changes {
			if change.Diff != nil {
				hasCounts = true
				totals.Added += change.Diff.Added
				totals.Removed += change.Diff.Removed
			}
		}
	}
	ui.Row(c).FillWidth().Padding(layout.Y(12), layout.X(layout.Density(12, 9))).
		Gap(layout.X(layout.Density(10, 6))).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(36, 36).Shrink(0).Radius(9).Background(side).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() { icon(c, glyph, muted, 19) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(viewport.Font(c, 14)).FontWeight(650)
			if hasCounts {
				preview.DiffCounts(c.Key("totals"), &totals)
			}
		})
		if len(changes) != 0 {
			label := locales.ArtifactViewChanges()
			button := pointerButton(c.Key("review"), false).Label(label).Shrink(0).
				MinHeight(layout.Target(30)).Padding(layout.Y(6), layout.X(9)).AlignItems(ui.Center).
				Radius(8).Border(1, border)
			if button.Hovered() {
				button.Background(side)
			}
			button.Children(func() { ui.Text(c, label).FontSize(viewport.Font(c, 13)).FontWeight(610) }).
				OnClick(func() { app.openRoundChanges(entry) })
		}
	})
}

func (app *chatApp) fileDeliverableRow(c *ui.Context, entry *turn, file preview.Change, workspace string) {
	layout := windowLayout(c)
	label := file.Path
	if rel, err := filepath.Rel(workspace, file.Path); workspace != "" && err == nil && filepath.IsLocal(rel) {
		label = rel
	}
	label = filepath.ToSlash(label)
	directory, name := "", label
	if slash := strings.LastIndexByte(label, '/'); slash >= 0 {
		directory, name = label[:slash+1], label[slash+1:]
	}
	path := file.Path
	quickLook := func() { app.showPreviewError(desktop.PreviewFile(path)) }
	open := quickLook
	if entry.Activity[file.Index].Tool == localtools.Write {
		open = func() { app.openToolPreview(entry, file.Index) }
	}

	button := pointerButton(c, false).Label(label).Tooltip(path).FillWidth().MinHeight(layout.Target(34)).
		Padding(layout.Y(5), layout.X(8)).Radius(6).Gap(layout.X(8)).AlignItems(ui.Center)
	if button.Hovered() {
		button.Background(side)
	}
	button.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Clip().Children(func() {
			if directory != "" {
				ui.Text(c, directory).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13)).TextColor(muted)
			}
			ui.Text(c, name).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13)).TextColor(ink)
		})
		if file.Diff != nil {
			preview.DiffCounts(c.Key("counts"), file.Diff)
		}
	}).OnClick(open)
	if !filepath.IsAbs(path) {
		return
	}
	items := []theme.PopupItem{{Label: desktop.PreviewLabel(), Icon: icons.Menu("open"), OnSelect: quickLook}}
	if !desktop.Mobile {
		items = append(items, theme.PopupItem{Label: locales.ArtifactReveal(), Icon: icons.Menu("folder_open"), OnSelect: func() {
			mygo.Shell.ShowItemInFolder(path)
		}})
	}
	theme.PopupContextMenu(button, items...)
}

func (app *chatApp) overlappingWorkspace(chat *conversation) bool {
	return chat != nil && chat.isRunning() && app.hasWorkspaceOverlap(chat.workspace, chat, "")
}

func (app *chatApp) hasWorkspaceOverlap(workspace string, exceptChat *conversation, exceptTask string) bool {
	if workspace == "" {
		return false
	}
	for _, other := range app.history.Chats {
		if other != exceptChat && other.isRunning() && workspacesOverlap(workspace, other.workspace) {
			return true
		}
	}
	for _, task := range app.backgroundRows {
		if task.Spec.ID != exceptTask && task.DoneAt == nil && workspacesOverlap(workspace, task.Directory) {
			return true
		}
	}
	return false
}

func workspacesOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && filepath.IsLocal(rel) {
			return true
		}
	}
	return false
}

type imageDeliverable struct {
	source string
	alt    string
}

func (entry *turn) deliverableImages() []imageDeliverable {
	if entry.State == "pending" || entry.Content == "" {
		return nil
	}
	if entry.artifactImageText == entry.Content {
		return entry.artifactImages
	}
	entry.artifactImageText = entry.Content
	entry.artifactImages = nil
	doc := parser.Parse(entry.Content)
	seen := make(map[string]bool)
	for block := 1; block < doc.Len(); block++ {
		if doc.Block(block).Kind != parser.Paragraph {
			continue
		}
		nodes := doc.Inlines(block)
		if !isImageParagraph(nodes) {
			continue
		}
		for i := 0; i < len(nodes); i = nodes[i].Next {
			node := nodes[i]
			if node.Kind != parser.Image || node.Dest == "" || node.Pending || seen[node.Dest] {
				continue
			}
			target, err := url.Parse(node.Dest)
			if err != nil || target.Hostname() == "" || (target.Scheme != "https" && target.Scheme != "http") {
				continue
			}
			seen[node.Dest] = true
			entry.artifactImages = append(entry.artifactImages, imageDeliverable{node.Dest, parser.PlainText(nodes[i+1 : node.Next])})
		}
	}
	return entry.artifactImages
}

func (app *chatApp) imageDeliverableRow(c *ui.Context, item imageDeliverable) {
	row := ui.Row(c).FillWidth().Padding(4, 8, 4, 11).Gap(7).AlignItems(ui.Center)
	image := app.scopedMarkdownImage(c, row, item.source)
	row.Children(func() {
		icon(c, "image", muted, 15)
		title := item.alt
		if title == "" {
			title = locales.ImagePreview()
		}
		ui.Text(c, title).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13)).FontWeight(600).Grow(1).MinWidth(0)
		select {
		case <-image.done:
			if image.bitmap != nil {
				taskIconButton(c.Key("preview"), locales.ImagePreview(), "open", func() {
					app.imagePreview = chatImagePreview{image: image, alt: item.alt}
				})
				taskIconButton(c.Key("download"), locales.DownloadImage(), "download", func() { app.downloadImage(image) })
			} else {
				ui.Text(c, locales.PreviewUnavailable()).FontSize(viewport.Font(c, 11)).TextColor(muted)
			}
		default:
			ui.Text(c, locales.ImagePreviewLoading()).FontSize(viewport.Font(c, 11)).TextColor(muted)
		}
	})
}
