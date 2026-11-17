package linkpreview

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"time"

	"github.com/ggdream/microlight/pkg/proxy"
	_ "golang.org/x/image/webp"
)

type Image struct {
	Pixels   image.Image
	Original []byte
	Format   string
}

// FetchImage loads a public raster image and retains its encoded bytes.
func FetchImage(ctx context.Context, raw string, config proxy.Config) (Image, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	transport, err := publicTransport(config)
	if err != nil {
		return Image{}, err
	}
	defer transport.CloseIdleConnections()
	res, err := fetchPublic(ctx, raw, "image/*", transport, previewTargetCheck(config))
	if err != nil {
		return Image{}, err
	}
	defer res.Body.Close()
	return decodeImage(res.Body)
}

func decodeImage(body io.Reader) (Image, error) {
	const maxBytes = 16 << 20
	const maxPixels = 16_000_000
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return Image{}, err
	}
	if len(data) > maxBytes {
		return Image{}, errors.New("image exceeds the download size limit")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Image{}, err
	}
	if config.Width < 1 || config.Height < 1 || config.Width > maxPixels/config.Height {
		return Image{}, errors.New("image exceeds the pixel limit or has invalid dimensions")
	}
	pixels, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Image{}, err
	}
	return Image{Pixels: pixels, Original: data, Format: format}, nil
}
