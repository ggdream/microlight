package preview

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/diff"

	"github.com/egoist/mygo/ui"
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

func (tree *Tree) View(c *ui.Context, roundKey any, icons asseticon.Set, selected int, open func(int)) {
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		ui.Text(c, locales.RoundChanges()).FontSize(12).FontWeight(650).TextColor(theme.Muted).Padding(9, 10, 5)
		if len(tree.children) == 0 {
			ui.Text(c, locales.NoRoundChanges()).FontSize(12).TextColor(theme.Muted).Padding(5, 10)
			return
		}
		ui.Scroll(c.Key(roundKey)).Grow(1).MinHeight(0).Children(func() {
			rows(c, icons, tree.children, 0, selected, open)
		})
	})
}

func rows(c *ui.Context, icons asseticon.Set, nodes []*Tree, depth, selected int, open func(int)) {
	for _, node := range nodes {
		rowContext := c.Key(node.key)
		inset := 8 + float32(min(depth, 8))*12
		if node.file == nil {
			folder := theme.PointerButton(rowContext, false).Label(node.name).FillWidth().MinHeight(28).
				AlignItems(ui.Center).Padding(3, 7, 3, inset).Gap(5).Radius(5)
			closed := ui.Local(folder, "closed", func() bool { return false })
			folder.Expanded(!*closed)
			if folder.Hovered() {
				folder.Background(theme.Side)
			}
			folder.Children(func() {
				arrow := ui.Icon(c, icons.SVG("ios_chevron_right")).Size(12, 12).Shrink(0).TextColor(theme.Muted)
				glyph := "folder"
				if !*closed {
					arrow.Rotate(90)
					glyph = "folder_open"
				}
				ui.Icon(c, icons.SVG(glyph)).Size(15, 15).Shrink(0).TextColor(theme.Muted)
				ui.Text(c, node.name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12)
			}).OnClick(func() { *closed = !*closed })
			if !*closed {
				rows(c, icons, node.children, depth+1, selected, open)
			}
			continue
		}
		file := theme.PointerButton(rowContext, false).Label(node.name).Tooltip(node.file.Path).
			FillWidth().MinHeight(28).AlignItems(ui.Center).Padding(3, 7, 3, inset+17).Gap(5).Radius(5)
		if selected == node.file.Index {
			file.Background(theme.Tint)
		} else if file.Hovered() {
			file.Background(theme.Side)
		}
		file.Children(func() {
			ui.Icon(c, icons.SVG(node.file.Icon)).Size(15, 15).Shrink(0).TextColor(theme.Muted)
			ui.Text(c, node.name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12)
			if node.file.Diff != nil {
				DiffCounts(c.Key("tree-counts"), node.file.Diff)
			}
		}).OnClick(func() { open(node.file.Index) })
	}
}
func DiffCounts(c *ui.Context, diff *diff.Lines) {
	ui.Row(c).Gap(5).Shrink(0).Children(func() {
		ui.Text(c, "+"+strconv.Itoa(diff.Added)).NoWrap().FontFeatures("tnum").TextColor(theme.Accent)
		ui.Text(c, "-"+strconv.Itoa(diff.Removed)).NoWrap().FontFeatures("tnum").TextColor(theme.ErrorFG)
	})
}
