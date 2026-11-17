package attachment

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ggdream/microlight/internal/storage"
)

// LocalPath returns the original absolute path or materializes an attachment
// without a source file, such as a pasted screenshot.
func (file *File) LocalPath() (string, error) {
	if filepath.IsAbs(file.Path) {
		return file.Path, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir, err := filepath.Abs(filepath.Join(cache, storage.AppDataDirName, "attachments"))
	if err != nil {
		return "", err
	}
	return file.cacheLocalPath(dir)
}

func (file *File) cacheLocalPath(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	ext := filepath.Ext(file.Name)
	if len(ext) > 12 {
		ext = ""
	}
	for _, ch := range strings.TrimPrefix(ext, ".") {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') {
			ext = ""
			break
		}
	}
	path := filepath.Join(dir, fmt.Sprintf("%x%s", sha256.Sum256(file.Data), ext))
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() == int64(len(file.Data)) {
		if data, err := os.ReadFile(path); err == nil && bytes.Equal(data, file.Data) {
			return path, nil
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temp, err := os.CreateTemp(dir, ".attachment-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(file.Data)
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		return "", err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}
