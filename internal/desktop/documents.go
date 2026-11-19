package desktop

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/ggdream/microlight/internal/storage"
)

// SaveDocument writes a file selected on desktop or exports a copy through
// the system document picker on mobile.
func SaveDocument(options mygo.SaveDialogOptions, data []byte) (string, error) {
	if Mobile {
		dir, err := os.MkdirTemp("", "microlight-export-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir)
		name := filepath.Base(options.DefaultPath)
		path := filepath.Join(dir, name)
		if err := storage.WritePrivateFile(path, data); err != nil {
			return "", err
		}
		return mygo.Dialog.SaveCopy(path, name)
	}
	path, err := mygo.Dialog.Save(options)
	if err != nil || path == "" {
		return path, err
	}
	return path, storage.WritePrivateFile(path, data)
}
