package storage

import (
	"os"
	"path/filepath"
)

// WritePrivateFile replaces a file only after its complete contents were written.
func WritePrivateFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".microlight-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
