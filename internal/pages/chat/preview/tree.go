package preview

import (
	"cmp"
	"github.com/ggdream/microlight/pkg/viewport"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/diff"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

type Change struct {
	Path, Icon string
	Index      int
	Diff       *diff.Lines
}

func (tree *Tree) HasChanges() bool { return len(tree.children) > 0 }

type Tree struct {
	name     string
	key      string
	children []*Tree
	file     *Change
}

func BuildTree(files []Change, workspace string) *Tree {
	root := &Tree{}
	folders := make(map[string]*Tree)
	for _, file := range files {
		path := file.Path
		var parts []string
		if relative, err := filepath.Rel(workspace, path); workspace != "" && err == nil && filepath.IsLocal(relative) {
			parts = strings.Split(relative, string(filepath.Separator))
		} else if filepath.IsAbs(path) {
			volume := filepath.VolumeName(path)
			prefix := volume + string(filepath.Separator)
			parts = append([]string{prefix}, strings.Split(strings.TrimPrefix(path, prefix), string(filepath.Separator))...)
		} else {
			parts = strings.Split(path, string(filepath.Separator))
		}
		parent := root
		for i, name := range parts[:len(parts)-1] {
			key := strings.Join(parts[:i+1], "\x00")
			child := folders[key]
			if child == nil {
				child = &Tree{name: name, key: key}
				folders[key] = child
				parent.children = append(parent.children, child)
			}
			parent = child
		}
		parent.children = append(parent.children, &Tree{
			name: parts[len(parts)-1], key: file.Path, file: &file,
		})
	}
	var sortChildren func(*Tree)
	sortChildren = func(node *Tree) {
		slices.SortFunc(node.children, func(a, b *Tree) int {
			if (a.file == nil) != (b.file == nil) {
				if a.file == nil {
					return -1
				}
				return 1
			}
			return cmp.Compare(a.name, b.name)
		})
		for _, child := range node.children {
			sortChildren(child)
		}
	}
	sortChildren(root)
	return root
}

func (tree *Tree) View(c *ui.Context, roundKey any, icons asseticon.Set, selected int, open func(int), quickLook func(string)) {
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		ui.Text(c, locales.RoundChanges()).FontSize(viewport.Font(c, 12)).FontWeight(650).TextColor(theme.Muted).Padding(9, 10, 5)
		if len(tree.children) == 0 {
			ui.Text(c, locales.NoRoundChanges()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted).Padding(5, 10)
			return
		}
		scroll, offset := elastic.Scroll(c.Key(roundKey))
		scroll.Grow(1).MinHeight(0).Children(func() {
			offset.Apply(ui.Themed(c, theme.PopupTheme(c)).Background(ui.Transparent).FillWidth()).Children(func() {
				rows(c, icons, tree.children, 0, selected, open, quickLook)
			})
		})
	})
}

func rows(c *ui.Context, icons asseticon.Set, nodes []*Tree, depth, selected int, open func(int), quickLook func(string)) {
	for _, node := range nodes {
		rowContext := c.Key(node.key)
		inset := treeInset(c, depth)
		if node.file == nil {
			if folderRow(rowContext, icons, node.name, "", inset, true) {
				rows(c, icons, node.children, depth+1, selected, open, quickLook)
			}
			continue
		}
		var preview func()
		if filepath.IsAbs(node.file.Path) {
			preview = func() { quickLook(node.file.Path) }
		}
		fileRow(rowContext, icons, node.name, *node.file, inset, selected == node.file.Index,
			func() { open(node.file.Index) }, preview)
	}
}

func treeInset(c *ui.Context, depth int) float32 {
	layout := viewport.Available(c)
	step := layout.Density(12, layout.X(10))
	return 8 + float32(min(depth, 8))*step
}

func folderRow(c *ui.Context, icons asseticon.Set, name, path string, inset float32, expanded bool) bool {
	folder := theme.PointerButton(c, false).Label(name).Tooltip(path).FillWidth().MinHeight(viewport.Available(c).Target(28)).
		AlignItems(ui.Center).Padding(3, 7, 3, inset).Gap(5).Radius(5)
	open := ui.Local(folder, "open", func() bool { return expanded })
	folder.Expanded(*open)
	if folder.Hovered() {
		folder.Background(theme.Hover)
	}
	folder.Children(func() {
		arrow := ui.Icon(c, icons.SVG("ios_chevron_right")).Size(12, 12).Shrink(0).TextColor(theme.Muted)
		glyph := "folder"
		if *open {
			arrow.Rotate(90)
			glyph = "folder_open"
		}
		ui.Icon(c, icons.SVG(glyph)).Size(15, 15).Shrink(0).TextColor(theme.Muted)
		ui.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 12))
	}).OnClick(func() { *open = !*open })
	return *open
}

func fileRow(c *ui.Context, icons asseticon.Set, name string, change Change, inset float32, selected bool, open, quickLook func()) {
	file := theme.PointerButton(c, open == nil).Label(name).Tooltip(change.Path).
		FillWidth().MinHeight(viewport.Available(c).Target(28)).AlignItems(ui.Center).Padding(3, 7, 3, inset+17).Gap(5).Radius(5)
	if selected {
		file.Background(theme.Tint)
	} else if file.Hovered() {
		file.Background(theme.Hover)
	}
	file.Children(func() {
		ui.Icon(c, icons.SVG(change.Icon)).Size(15, 15).Shrink(0).TextColor(theme.Muted)
		ui.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 12))
		if change.Diff != nil {
			DiffCounts(c.Key("tree-counts"), change.Diff)
		}
	})
	if open != nil {
		file.OnClick(open)
	}
	if quickLook != nil {
		if file.Shortcut(0, ui.KeySpace) {
			quickLook()
		}
		theme.PopupContextMenu(file, theme.PopupItem{Label: desktop.PreviewLabel(), Icon: icons.Menu("open"), OnSelect: quickLook})
	}
}

func DiffCounts(c *ui.Context, diff *diff.Lines) {
	ui.Row(c).Gap(5).Shrink(0).Children(func() {
		ui.Text(c, "+"+strconv.Itoa(diff.Added)).NoWrap().FontFeatures("tnum").TextColor(ui.Hex("#256B35"))
		ui.Text(c, "-"+strconv.Itoa(diff.Removed)).NoWrap().FontFeatures("tnum").TextColor(theme.ErrorFG)
	})
}
