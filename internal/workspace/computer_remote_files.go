package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"uuid"

	"github.com/cloudwego/eino/adk/filesystem"
)

type desktopFiles struct {
	directory string
	identity  os.FileInfo
	writeMu   sync.Mutex
}

func desktopEnvironment() Environment { return Environment{OS: runtime.GOOS, Arch: runtime.GOARCH} }

func (f *desktopFiles) commandDirectory(ctx context.Context, name string) (string, error) {
	result, err := f.execute(ctx, "resolve", "", desktopFileRequest{Path: name})
	if err != nil {
		return "", err
	}
	if !result.(desktopFileReply).IsDir {
		return "", errors.New("desktop command directory must be a directory")
	}
	return filepath.Join(f.directory, filepath.FromSlash(strings.TrimPrefix(name, "/"))), nil
}

func openDesktopFiles(directory string) (*desktopFiles, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("desktop file root must be a directory")
	}
	return &desktopFiles{directory: directory, identity: info}, nil
}

type desktopFileRequest struct {
	Path, Content, Version string
	Write                  bool
	Offset, Limit          int
}

type desktopFileReply struct {
	Path, Version string
	IsDir         bool
}

func (f *desktopFiles) serve(ctx context.Context, w http.ResponseWriter, r *http.Request, method, session string) {
	var args desktopFileRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid file arguments", http.StatusBadRequest)
		return
	}
	result, err := f.execute(ctx, method, session, args)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (f *desktopFiles) execute(ctx context.Context, method, session string, args desktopFileRequest) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := desktopRelativePath(args.Path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(f.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil || !os.SameFile(info, f.identity) {
		return nil, errors.New("desktop file root changed")
	}
	if method == "write" {
		f.writeMu.Lock()
		defer f.writeMu.Unlock()
	}
	info, err = desktopFileInfo(root, name)
	if errors.Is(err, os.ErrNotExist) && info == nil && (method == "write" || method == "resolve" && args.Write) {
		if _, parentErr := desktopFileInfo(root, path.Dir(name)); parentErr == nil {
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}
	switch method {
	case "resolve":
		version, err := desktopFileVersion(root, name, session, info, args.Write)
		return desktopFileReply{Path: args.Path, Version: version, IsDir: info != nil && info.IsDir()}, err
	case "read":
		if !info.Mode().IsRegular() {
			return nil, errors.New("desktop path is not a regular file")
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		return readText(file, &filesystem.ReadRequest{Offset: args.Offset, Limit: args.Limit})
	case "list":
		if !info.IsDir() {
			return nil, errors.New("desktop path is not a directory")
		}
		dir, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		defer dir.Close()
		names, err := dir.Readdirnames(200)
		if err != nil && err != io.EOF {
			return nil, err
		}
		entries := make([]filesystem.FileInfo, 0, len(names))
		for _, name := range names {
			entries = append(entries, filesystem.FileInfo{Path: name})
		}
		return entries, nil
	case "write":
		return nil, writeDesktopFile(ctx, root, name, session, info, args)
	default:
		return nil, errors.New("unknown desktop file operation")
	}
}

func desktopRelativePath(name string) (string, error) {
	if name == "/" {
		return ".", nil
	}
	relative, rooted := strings.CutPrefix(name, "/")
	if !rooted || !filepath.IsLocal(relative) || path.Clean(relative) != relative ||
		relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || strings.ContainsAny(relative, "\\:\x00") {
		return "", errors.New("invalid desktop workspace path")
	}
	return relative, nil
}

func desktopFileInfo(root *os.Root, name string) (os.FileInfo, error) {
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		info, err := root.Lstat(parent)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errors.New("desktop file parent must be a directory")
		}
	}
	info, err := root.Lstat(name)
	if err == nil && !info.IsDir() && !info.Mode().IsRegular() {
		return nil, errors.New("desktop paths must be regular files or directories")
	}
	return info, err
}

func desktopFileVersion(root *os.Root, name, session string, info os.FileInfo, write bool) (string, error) {
	if info == nil {
		return "missing", nil
	}
	version := fmt.Sprintf("%s:%d:%d:%d", session, info.Size(), info.ModTime().UnixNano(), info.Mode())
	if !write {
		return version, nil
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("desktop path is not a regular file")
	}
	if info.Size() <= fileLimit {
		file, err := root.Open(name)
		if err != nil {
			return "", err
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, io.LimitReader(file, fileLimit+1)); err != nil {
			return "", err
		}
		version += fmt.Sprintf(":%x", hash.Sum(nil))
	}
	return version, nil
}

func writeDesktopFile(ctx context.Context, root *os.Root, name, session string, info os.FileInfo, args desktopFileRequest) error {
	if len(args.Content) > 16<<10 || strings.ContainsRune(args.Content, 0) {
		return errors.New("invalid desktop file content")
	}
	version, err := desktopFileVersion(root, name, session, info, true)
	if err != nil {
		return err
	}
	if args.Version != version {
		return errors.New("desktop file changed")
	}
	mode := os.FileMode(0600)
	if info != nil {
		mode = info.Mode().Perm()
	}
	temp := path.Join(path.Dir(name), ".microlight-"+uuid.NewV7().String())
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, err = file.WriteString(args.Content)
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if info == nil {
		return root.Link(temp, name)
	}
	current, err := desktopFileInfo(root, name)
	if err != nil {
		return err
	}
	currentVersion, err := desktopFileVersion(root, name, session, current, true)
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) || currentVersion != version {
		return errors.New("desktop file changed")
	}
	return root.Rename(temp, name)
}

type remoteDesktopFiles struct {
	desktop   *remoteDesktop
	directory string
}

func (d *remoteDesktop) Files() ResolvedFileSystem {
	if !d.files {
		return nil
	}
	return remoteDesktopFiles{desktop: d, directory: d.directory}
}

func (f remoteDesktopFiles) Resolve(ctx context.Context, name string, write bool) (string, string, error) {
	if !path.IsAbs(name) {
		name = path.Join(f.directory, name)
	}
	var result desktopFileReply
	err := f.desktop.call(ctx, "files/resolve", desktopFileRequest{Path: path.Clean(name), Write: write}, &result)
	return result.Path, result.Version, err
}

func (f remoteDesktopFiles) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	var entries []filesystem.FileInfo
	err := f.desktop.call(ctx, "files/list", desktopFileRequest{Path: req.Path}, &entries)
	return entries, err
}

func (f remoteDesktopFiles) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	var result filesystem.FileContent
	err := f.desktop.call(ctx, "files/read", desktopFileRequest{Path: req.FilePath, Offset: req.Offset, Limit: req.Limit}, &result)
	return &result, err
}

func (f remoteDesktopFiles) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	version, err := writeVersion(ctx, f, req.FilePath)
	if err != nil {
		return err
	}
	return f.desktop.call(ctx, "files/write", desktopFileRequest{Path: req.FilePath, Content: req.Content, Version: version}, nil)
}
