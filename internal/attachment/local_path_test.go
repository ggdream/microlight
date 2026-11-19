package attachment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttachmentUsesSavedBytesWhenPickerCopyDisappears(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "picked.txt")
	data := []byte("Content from picker fixture " + dir)
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := Load(source)
	if err != nil {
		t.Fatal(err)
	}
	if path, err := file.LocalPath(); err != nil || path != source {
		t.Fatalf("expected the available original file, got %q: %v", path, err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	path, err := file.LocalPath()
	if err != nil || !filepath.IsAbs(path) || path == source {
		t.Fatalf("expected an absolute path to the cached copy, got %q: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	if cached, err := os.ReadFile(path); err != nil || string(cached) != string(data) {
		t.Fatalf("the cached copy did not preserve the attachment: %v", err)
	}
}

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
