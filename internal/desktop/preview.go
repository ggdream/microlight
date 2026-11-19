package desktop

import (
	"errors"
	"os"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/ggdream/microlight/assets/locales"
)

func PreviewLabel() string {
	if Mobile {
		return locales.ShareFile()
	}
	if runtime.GOOS == "darwin" {
		return locales.QuickLookFile()
	}
	return locales.OpenWithDefaultApp()
}

func PreviewFile(path string) error {
	file, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !file.Mode().IsRegular() {
		return errors.New("preview requires a regular file")
	}
	if runtime.GOOS == "darwin" {
		return mygo.Shell.PreviewItem(path)
	}
	return mygo.Shell.OpenPath(path)
}
