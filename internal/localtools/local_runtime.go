package localtools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/ggdream/microlight/internal/shell"
	"github.com/ggdream/microlight/internal/workspace"
	"github.com/ggdream/microlight/pkg/diff"
)

var errLocalCommandDirectory = errors.New("命令的工作目录不可用")

// LocalRuntime uses native file and process operations. Browser and desktop
// drivers are borrowed from the host so their identities survive across turns.
type LocalRuntime struct {
	Directory     string
	BrowserDriver workspace.BrowserDriver
	DesktopDriver workspace.ComputerDriver
}

func (r LocalRuntime) Environment() workspace.Environment {
	name := "/bin/bash"
	if runtime.GOOS == "windows" {
		name = "PowerShell"
	}
	return workspace.Environment{OS: runtime.GOOS, Arch: runtime.GOARCH, Shell: name}
}

func (r LocalRuntime) Close() error                       { return nil }
func (r LocalRuntime) Browser() workspace.BrowserDriver   { return r.BrowserDriver }
func (r LocalRuntime) Computer() workspace.ComputerDriver { return r.DesktopDriver }

func (r LocalRuntime) Resolve(_ context.Context, path string, write bool) (string, string, error) {
	name := Read
	if write {
		name = Write
	}
	op, err := r.fileOperation(name, path, "")
	if err != nil {
		return "", "", err
	}
	if write && !op.exists {
		return op.target, "missing", nil
	}
	return op.target, op.fileVersion, nil
}

func (r LocalRuntime) fileOperation(name, path, content string) (localOperation, error) {
	return (Middleware{Workspace: r.Directory}).resolveLocalFile(name, localArguments{Path: path, Content: &content})
}

func (r LocalRuntime) executeApprovedFile(ctx context.Context, op localOperation) (string, *diff.Lines, error) {
	return executeFiles(ctx, localFilesystem{operation: op}, op)
}

func (r LocalRuntime) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	op, err := r.fileOperation(List, req.Path, "")
	if err != nil {
		return nil, err
	}
	resolved := *req
	resolved.Path = op.target
	return (localFilesystem{operation: op}).LsInfo(ctx, &resolved)
}

func (r LocalRuntime) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	op, err := r.fileOperation(Read, req.FilePath, "")
	if err != nil {
		return nil, err
	}
	resolved := *req
	resolved.FilePath = op.target
	return (localFilesystem{operation: op}).Read(ctx, &resolved)
}

func (r LocalRuntime) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	op, err := r.fileOperation(Write, req.FilePath, req.Content)
	if err != nil {
		return err
	}
	version := op.fileVersion
	if !op.exists {
		version = "missing"
	}
	if err := workspace.CheckWriteVersion(ctx, op.target, version); err != nil {
		return err
	}
	if op.target != req.FilePath {
		return errors.New("文件路径已变化")
	}
	return (localFilesystem{operation: op}).Write(ctx, req)
}

func (r LocalRuntime) Execute(ctx context.Context, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	dir, err := localCommandDirectory(r.Directory)
	if err != nil {
		return nil, err
	}
	return r.executeApprovedCommand(ctx, dir, req)
}

func (r LocalRuntime) executeApprovedCommand(ctx context.Context, dir string, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	return shell.Execute(ctx, req.Command, dir, req.Timeout)
}

func localCommandDirectory(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() || !filepath.IsAbs(resolved) {
		return "", errLocalCommandDirectory
	}
	return resolved, nil
}
