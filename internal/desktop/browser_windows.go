package desktop

import (
	"context"

	"golang.org/x/sys/windows"
)

// OpenBrowser hands a validated HTTP(S) URL to the desktop's default browser.
func OpenBrowser(ctx context.Context, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	url, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, nil, url, nil, nil, windows.SW_SHOWNORMAL)
}
