package chat

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/desktop"
)

func (app *chatApp) quickLookAttachment(file *attachment.File) error {
	if desktop.Web {
		return desktop.PreviewBrowserFile(file.Data, file.MIME)
	}
	if app.previewDirectory == "" {
		directory, err := os.MkdirTemp("", "microlight-preview-")
		if err != nil {
			return err
		}
		app.previewDirectory = directory
	}
	directory, err := os.MkdirTemp(app.previewDirectory, "file-")
	if err != nil {
		return err
	}
	name := filepath.Base(file.Name)
	if !filepath.IsLocal(name) || name == "." {
		name = "attachment"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, file.Data, 0o600); err != nil {
		return err
	}
	return desktop.PreviewFile(path)
}

func (app *chatApp) showPreviewError(err error) {
	if err != nil {
		slog.Error("preview file", "error", err)
		go func() {
			_, _ = app.showMessage(mygo.MessageOptions{Parent: app.win, Type: mygo.MessageWarning, Message: locales.OpenFileFailed()})
		}()
	}
}
