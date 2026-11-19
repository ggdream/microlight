//go:build !windows

package desktop

import (
	"context"
	"errors"
	"github.com/egoist/mygo"
	"os/exec"
	"runtime"
)

// OpenBrowser hands a validated HTTP(S) URL to the desktop's default browser.
func OpenBrowser(ctx context.Context, target string) error {
	switch runtime.GOOS {
	case "ios", "android", "js":
		return mygo.Shell.OpenExternal(target)
	case "darwin":
		return exec.CommandContext(ctx, "/usr/bin/open", target).Run()
	case "linux":
		return exec.CommandContext(ctx, "xdg-open", target).Run()
	default:
		return errors.New("opening the default browser is not supported on this platform")
	}
}
