//go:build !js

package desktop

import "errors"

func OpenBrowserFiles(bool, string, int) ([]BrowserFile, error) {
	return nil, errors.New("browser file picker is unavailable")
}
func DownloadBrowserFile(string, []byte) error { return errors.New("browser download is unavailable") }
func PreviewBrowserFile([]byte, string) error  { return errors.New("browser preview is unavailable") }
