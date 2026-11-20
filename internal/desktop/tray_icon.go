package desktop

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
)

// Remove desktop launcher padding before the OS downsamples to tray size.
func cropTrayIcon(data []byte) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var bounds image.Rectangle
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha >= 0x8000 {
				bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if bounds.Empty() {
		return nil, errors.New("empty tray icon")
	}
	icon := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(icon, icon.Bounds(), source, bounds.Min, draw.Src)
	var output bytes.Buffer
	err = png.Encode(&output, icon)
	return output.Bytes(), err
}
