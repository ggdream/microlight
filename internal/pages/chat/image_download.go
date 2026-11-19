package chat

import (
	"log/slog"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/linkpreview"
)

func imageFilename(source, format string) string {
	ext := "." + format
	if format == "jpeg" {
		ext = ".jpg"
	}
	if u, ok := linkpreview.URL(source); ok {
		name := path.Base(u.Path)
		sourceExt := strings.ToLower(path.Ext(name))
		if (sourceExt == ext || format == "jpeg" && sourceExt == ".jpeg") && len(name) <= 180 &&
			filepath.IsLocal(name) && !strings.HasPrefix(name, ".") &&
			!strings.ContainsAny(name, `<>:"/\|?*`) && strings.IndexFunc(name, unicode.IsControl) == -1 {
			return name
		}
	}
	return "image" + ext
}

func (app *chatApp) downloadImage(image *markdownImage) {
	if app.win == nil || !app.savingImage.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer app.savingImage.Store(false)
		dir, _ := mygo.App.Path(mygo.PathDownloads)
		label := locales.DownloadImage()
		_, err := desktop.SaveDocument(mygo.SaveDialogOptions{
			Parent: app.win, Title: label, DefaultPath: filepath.Join(dir, image.filename),
			CreateDirectories: true,
			Filters:           []mygo.FileFilter{{Name: locales.ImageFilter(), Extensions: []string{strings.TrimPrefix(filepath.Ext(image.filename), ".")}}},
		}, image.original)
		if err != nil {
			slog.Error("save chat image", "error", err)
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Parent: app.win, Type: mygo.MessageError, Message: locales.DownloadImageFailed(),
			})
		}
	}()
}
