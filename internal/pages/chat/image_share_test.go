package chat

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedImageKeepsOriginalFileForSystemReader(t *testing.T) {
	cache := t.TempDir()
	original := []byte("original encoded image bytes\x00\xff")
	path, err := prepareSharedImage(cache, "../../original.png", original)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cache, path)
	if err != nil || !filepath.IsLocal(relative) || filepath.Base(path) != "original.png" {
		t.Fatalf("shared image escaped the cache or lost its filename: %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("system reader did not receive original bytes: %v", err)
	}
	again, err := prepareSharedImage(cache, "original.png", original)
	if err != nil || again != path {
		t.Fatalf("sharing the same image should reuse its cached path: %q %v", again, err)
	}
	other, err := prepareSharedImage(cache, "original.png", []byte("different image"))
	if err != nil || other == path {
		t.Fatalf("different images with the same name must not overwrite each other: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("an earlier shared image was lost before the system read it: %v", err)
	}
}
