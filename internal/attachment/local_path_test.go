package attachment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttachmentWithoutSourceHasReusableLocalPath(t *testing.T) {
	dir := t.TempDir()
	file := &File{Name: "Clipboard image.png", Data: []byte("image bytes")}
	path, err := file.cacheLocalPath(dir)
	if err != nil || filepath.Dir(path) != dir || !filepath.IsAbs(path) || filepath.Ext(path) != ".png" {
		t.Fatalf("unexpected cache path %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("edited bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := file.cacheLocalPath(dir)
	if err != nil || again != path {
		t.Fatalf("cache path changed: %q, %v", again, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(file.Data) {
		t.Fatalf("cached bytes differ from the attachment: %v", err)
	}
}
