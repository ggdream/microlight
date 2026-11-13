package workspace

import (
	"os"
	"path/filepath"
)

// Directories returns the selected directory first, stopping at the
// nearest repository root (including worktrees). Outside Git, use just the
// selected directory rather than loading instructions from unrelated ancestors.
func Directories(workspace string) []string {
	if !filepath.IsAbs(workspace) {
		return nil
	}
	start := filepath.Clean(workspace)
	var dirs []string
	for dir := start; ; dir = filepath.Dir(dir) {
		dirs = append(dirs, dir)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dirs
		}
		if filepath.Dir(dir) == dir {
			return dirs[:1]
		}
	}
}
