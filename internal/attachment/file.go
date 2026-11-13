package attachment

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	MaxPerMessage        = 8
	MaxBytes             = 8 << 20
	MaxDraftBytes        = 32 << 20
	MaxRequestImageBytes = 32 << 20
)

type File struct {
	Name     string
	MIME     string
	Data     []byte
	thumb    *ui.Bitmap
	textOnce sync.Once
	textFile bool
}

func Load(path string) (*File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.New(locales.OpenFileFailed())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New(locales.ChooseRegularFile())
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New(locales.OpenFileFailed())
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		return nil, errors.New(locales.ReadFileFailed())
	}
	return New(filepath.Base(path), data)
}

func New(name string, data []byte) (*File, error) {
	if len(data) > MaxBytes {
		return nil, errors.New(locales.AttachmentTooLarge())
	}
	attachment := &File{Name: name, MIME: http.DetectContentType(data), Data: data}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		if attachment.IsImage() {
			return nil, errors.New(locales.CorruptImage())
		}
		return attachment, nil
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
		return nil, errors.New(locales.TooManyPixels())
	}
	attachment.MIME = "image/" + format
	if attachment.Thumbnail() == nil {
		return nil, errors.New(locales.CorruptImage())
	}
	return attachment, nil
}

func (attachment *File) IsImage() bool {
	switch attachment.MIME {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (attachment *File) IsText() bool {
	attachment.textOnce.Do(func() {
		if !strings.HasPrefix(attachment.MIME, "text/") || !utf8.Valid(attachment.Data) {
			return
		}
		for _, r := range string(attachment.Data) {
			if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
				return
			}
		}
		attachment.textFile = true
	})
	return attachment.textFile
}

func (attachment *File) Thumbnail() *ui.Bitmap {
	if attachment.thumb != nil {
		return attachment.thumb
	}
	src, _, err := image.Decode(bytes.NewReader(attachment.Data))
	if err != nil {
		return nil
	}
	size := src.Bounds().Size()
	const edge = 192
	scale := min(1.0, float64(edge)/float64(max(size.X, size.Y)))
	thumb := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(size.X)*scale)), max(1, int(float64(size.Y)*scale))))
	draw.ApproxBiLinear.Scale(thumb, thumb.Bounds(), src, src.Bounds(), draw.Src, nil)
	attachment.thumb = ui.NewBitmap(thumb)
	return attachment.thumb
}
