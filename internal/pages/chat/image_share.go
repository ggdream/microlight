package chat

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/storage"
)

// Share returns when the system sheet appears, before another app reads the
// file. Keep the original bytes in the app's cache instead of removing a temp
// file as soon as Share returns. Repeated shares reuse the same content path.
func prepareSharedImage(cache, name string, data []byte) (string, error) {
	name = filepath.Base(name)
	if !filepath.IsAbs(cache) || !filepath.IsLocal(name) || name == "." || len(data) == 0 {
		return "", errors.New("invalid image to share")
	}
	directory := filepath.Join(cache, "shared-images", fmt.Sprintf("%x", sha256.Sum256(data)))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, name)
	return path, storage.WritePrivateFile(path, data)
}

func (app *chatApp) shareImage(name string, data []byte) {
	if !desktop.Mobile || app.win == nil || !app.savingImage.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer app.savingImage.Store(false)
		cache, err := mygo.App.Path(mygo.PathCache)
		if err == nil {
			var path string
			path, err = prepareSharedImage(cache, name, data)
			if err == nil {
				err = mygo.Share(mygo.ShareOptions{Files: []string{path}})
			}
		}
		if err != nil {
			slog.Error("share chat image", "error", err)
			_, _ = app.showMessage(mygo.MessageOptions{
				Parent: app.win, Type: mygo.MessageError, Message: locales.ShareImageFailed(),
			})
		}
	}()
}
