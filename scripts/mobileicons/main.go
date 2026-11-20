// Generate mobile packaging assets from the existing desktop logo. This removes
// transparent desktop insets, rather than changing the logo or its artwork.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	file, err := os.Open("assets/icon.png")
	if err != nil {
		return err
	}
	source, err := png.Decode(file)
	file.Close()
	if err != nil {
		return err
	}
	content := image.Rectangle{}
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			_, _, _, a := source.At(x, y).RGBA()
			if a >= 0xfa00 {
				content = content.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if content.Empty() || content.Dx() != content.Dy() {
		return fmt.Errorf("expected a nonempty square logo, got %v", content)
	}
	// Extend edge colors through the desktop logo's transparent rounded corners.
	// Native mobile launchers apply their own shape to this opaque square.
	square := image.NewNRGBA(image.Rect(0, 0, content.Dx(), content.Dy()))
	known := make([]bool, content.Dx()*content.Dy())
	for y := 0; y < square.Bounds().Dy(); y++ {
		left, right := -1, -1
		for x := 0; x < square.Bounds().Dx(); x++ {
			_, _, _, alpha := source.At(content.Min.X+x, content.Min.Y+y).RGBA()
			known[y*content.Dx()+x] = alpha >= 0xfa00
			_, _, _, a := source.At(content.Min.X+x, content.Min.Y+y).RGBA()
			if a >= 0xfa00 {
				if left < 0 {
					left = x
				}
				right = x
			}
		}
		if left < 0 {
			return fmt.Errorf("empty content row %d", y)
		}
		for x := 0; x < square.Bounds().Dx(); x++ {
			pixel := color.NRGBAModel.Convert(source.At(content.Min.X+max(left, min(x, right)), content.Min.Y+y)).(color.NRGBA)
			pixel.A = 255
			square.SetNRGBA(x, y, pixel)
		}
	}
	smoothCorners(square, known)
	mobile := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	draw.CatmullRom.Scale(mobile, mobile.Bounds(), square, square.Bounds(), draw.Src, nil)
	if err := writePNG("assets/icon-mobile.png", mobile); err != nil {
		return err
	}
	// Android layers are 108dp, with a central 72dp viewport. Keep the full
	// mobile logo in that viewport and extend its colors into the outer layer.
	adaptive := image.NewNRGBA(image.Rect(0, 0, 1536, 1536))
	for y := 0; y < 1536; y++ {
		for x := 0; x < 1536; x++ {
			adaptive.SetNRGBA(x, y, mobile.NRGBAAt(max(0, min(x-256, 1023)), max(0, min(y-256, 1023))))
		}
	}
	return writePNG("assets/icon-android.png", adaptive)
}

// Diffuse neighboring edge colors only through missing corner pixels. Known
// artwork stays untouched, and the extension has no row-shaped color bands.
func smoothCorners(img *image.NRGBA, known []bool) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	values := make([][3]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := img.NRGBAAt(x, y)
			values[y*w+x] = [3]float32{float32(p.R), float32(p.G), float32(p.B)}
		}
	}
	for pass := 0; pass < 400; pass++ {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if known[y*w+x] {
					continue
				}
				neighbors := [4]int{y*w + max(0, x-1), y*w + min(w-1, x+1), max(0, y-1)*w + x, min(h-1, y+1)*w + x}
				for channel := 0; channel < 3; channel++ {
					var total float32
					for _, n := range neighbors {
						total += values[n][channel]
					}
					values[y*w+x][channel] = total / 4
				}
			}
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !known[y*w+x] {
				v := values[y*w+x]
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(v[0] + .5), G: uint8(v[1] + .5), B: uint8(v[2] + .5), A: 255})
			}
		}
	}
}

func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(file, img)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
