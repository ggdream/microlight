//go:build !windows

package workspace

import (
	"context"
	"os/exec"
	"syscall"
)

const desktopShellName = "/bin/sh"

func desktopShellCommand(command string) *exec.Cmd {
	return exec.Command(desktopShellName, "-c", command)
}

func runDesktopProcess(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return waitDesktopProcess(ctx, cmd, func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
}
