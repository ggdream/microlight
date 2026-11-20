package desktop

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestTrayIconRemovesLauncherPadding(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 20; y < 80; y++ {
		for x := 20; x < 80; x++ {
			source.Set(x, y, color.NRGBA{R: 120, A: 255})
		}
	}
	// Keep the rounded transparent corner within the cropped artwork.
	source.Set(20, 20, color.NRGBA{})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	data, err := cropTrayIcon(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 60 || got.Bounds().Dy() != 60 {
		t.Fatalf("cropped bounds = %v", got.Bounds())
	}
	_, _, _, alpha := got.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatal("rounded corner was lost")
	}
}
