package linkpreview

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/proxy"
)

func TestDecodePreviewImage(t *testing.T) {
	want := image.Rect(0, 0, 4, 3)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(want)); err != nil {
		t.Fatal(err)
	}
	got, err := decodeImage(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Pixels.Bounds() != want {
		t.Fatalf("image bounds = %v, want %v", got.Pixels.Bounds(), want)
	}
	if got.Format != "png" || !bytes.Equal(got.Original, data.Bytes()) {
		t.Fatal("original PNG bytes were not preserved")
	}
	if _, err := decodeImage(strings.NewReader("<html>access denied</html>")); err == nil {
		t.Fatal("accepted a non-image response")
	}
}

func TestFetchPreviewImageRejectsLocalURL(t *testing.T) {
	if _, err := FetchImage(t.Context(), "http://127.0.0.1/test.png", proxy.Config{}); err == nil || !strings.Contains(err.Error(), "destination is local") {
		t.Fatalf("local destination was not rejected: %v", err)
	}
}
