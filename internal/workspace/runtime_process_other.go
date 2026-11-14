//go:build !windows

package workspace

import "os/exec"

func hideCommandWindow(*exec.Cmd) {}
