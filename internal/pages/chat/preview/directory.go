package preview

import (
	"cmp"
	"encoding/json"
	"github.com/ggdream/microlight/pkg/viewport"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/elastic"
)

const directoryEntryLimit = 200

type Directory struct {
	path     string
	selected string
	captured []string
	folders  map[string]*directoryListing
}

type directoryListing struct {
	entries   []os.DirEntry
	pending   bool
	failed    bool
	truncated bool
}

func NewDirectory(path string, output *string) *Directory {
	dir := &Directory{path: path, folders: make(map[string]*directoryListing)}
	if output != nil && json.Unmarshal([]byte(*output), &dir.captured) != nil {
		dir.captured = nil
	}
	return dir
}

func (dir *Directory) View(c *ui.Context, icons asseticon.Set, name string, update func(func()), open func(string)) {
	scroll, offset := elastic.Scroll(c)
	scroll.Grow(1).MinHeight(0).Children(func() {
		offset.Apply(ui.Themed(c, theme.PopupTheme(c)).Background(ui.Transparent).FillWidth().Padding(5, 4)).Children(func() {
			if folderRow(c.Key("root"), icons, name, dir.path, treeInset(c, 0), true) {
				dir.branch(c.Key("entries"), icons, dir.path, 0, update, open)
			}
		})
	})
}

func (dir *Directory) branch(c *ui.Context, icons asseticon.Set, path string, depth int, update func(func()), open func(string)) {
	branch := ui.Column(c).FillWidth().MinWidth(0)
	if depth < 8 {
		inset := treeInset(c, depth)
		branch.Draw(func(p *ui.Painter, box ui.Rect) {
			p.Fill(ui.Rect{X: box.X + inset + 6, Y: box.Y, W: 1, H: box.H}, theme.Border, 0)
		})
	}
	branch.Children(func() { dir.rows(c, icons, path, depth+1, update, open) })
}

func (dir *Directory) rows(c *ui.Context, icons asseticon.Set, path string, depth int, update func(func()), open func(string)) {
	listing := dir.load(path, update)
	inset := treeInset(c, depth)
	note := func(text string) {
		ui.Text(c, text).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted).Padding(5, 7, 5, inset+17)
	}
	switch {
	case listing.pending:
		note(locales.DirectoryLoading())
	case listing.failed:
		note(locales.PreviewUnavailable())
		if path == dir.path {
			for i, name := range dir.captured {
				fileRow(c.Key(i), icons, name, Change{Icon: FileIcon(name)}, inset, false, nil, nil)
			}
		}
	case len(listing.entries) == 0:
		note(locales.DirectoryEmpty())
	default:
		for _, entry := range listing.entries {
			name := entry.Name()
			child := filepath.Join(path, name)
			row := c.Key(child)
			if entry.IsDir() {
				if folderRow(row, icons, name, child, inset, false) {
					dir.branch(row.Key("contents"), icons, child, depth, update, open)
				}
				continue
			}
			preview := func() {
				dir.selected = child
				open(child)
			}
			fileRow(row, icons, name, Change{Path: child, Icon: FileIcon(child)}, inset, dir.selected == child, preview, preview)
		}
		if listing.truncated {
			note(locales.DirectoryEntryLimit())
		}
	}
}

func (dir *Directory) load(path string, update func(func())) *directoryListing {
	if listing := dir.folders[path]; listing != nil {
		return listing
	}
	listing := &directoryListing{pending: true}
	dir.folders[path] = listing
	if !filepath.IsAbs(dir.path) {
		*listing = directoryListing{failed: true}
		return listing
	}
	if update == nil {
		*listing = readDirectory(dir.path, path)
		return listing
	}
	go func() {
		result := readDirectory(dir.path, path)
		update(func() { *listing = result })
	}()
	return listing
}

func readDirectory(base, path string) directoryListing {
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return directoryListing{failed: true}
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return directoryListing{failed: true}
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return directoryListing{failed: true}
	}
	defer file.Close()
	entries, err := file.ReadDir(directoryEntryLimit + 1)
	if err != nil && err != io.EOF {
		return directoryListing{failed: true}
	}
	truncated := len(entries) > directoryEntryLimit
	if truncated {
		entries = entries[:directoryEntryLimit]
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name())), cmp.Compare(a.Name(), b.Name()))
	})
	return directoryListing{entries: entries, truncated: truncated}
}

func FileIcon(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".go", ".js", ".ts", ".jsx", ".tsx", ".css", ".html", ".json", ".py", ".rs", ".sh":
		return "code"
	default:
		return "document_text"
	}
}
