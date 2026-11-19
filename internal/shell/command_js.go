//go:build js

package shell

import (
	"context"
	"os/exec"
)

// Browser agents do not register shell tools. Keep the package compilable for
// shared types; os/exec reports unsupported if this is accidentally invoked.
func platformShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "unsupported-browser-shell", command)
}
