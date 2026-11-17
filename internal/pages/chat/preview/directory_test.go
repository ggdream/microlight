package preview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	asseticon "github.com/ggdream/microlight/internal/icon"
)

func TestDirectoryExpandsFolderAndPreviewsFile(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "nested")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "sample.png")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	dir := NewDirectory(root, nil)
	var opened string
	window := ui.NewTester(func(c *ui.Context) {
		dir.View(c, asseticon.Set{}, "root", nil, func(path string) { opened = path })
	}, 300, 250)
	if window.HasText("sample.png") || dir.folders[folder] != nil {
		t.Fatal("collapsed folder was loaded")
	}
	nested, ok := window.Find("nested")
	if !ok {
		t.Fatal("folder row is missing")
	}
	window.ClickAt(nested.X+nested.W/2, nested.Y+nested.H/2)
	item, ok := window.Find("sample.png")
	if !ok {
		t.Fatal("expanded folder did not show its contents")
	}
	window.ClickAt(item.X+item.W/2, item.Y+item.H/2)
	if opened != file {
		t.Fatalf("preview opened %q, want %q", opened, file)
	}
	window.ClickAt(nested.X+nested.W/2, nested.Y+nested.H/2)
	if window.HasText("sample.png") {
		t.Fatal("collapsed folder still shows its contents")
	}
}
