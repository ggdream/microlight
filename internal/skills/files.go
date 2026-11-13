package skills

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

// skillFiles presents a single discovered manifest to Eino's filesystem loader.
type skillFiles struct{ dir, path string }

func (s skillFiles) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	if req.Path != s.dir || req.Pattern != "*/SKILL.md" {
		return nil, errors.ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []filesystem.FileInfo{{Path: s.path}}, nil
}

func (s skillFiles) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.FilePath != s.path {
		return nil, fs.ErrPermission
	}
	root, err := os.OpenRoot(filepath.Dir(s.path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open("SKILL.md")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", s.path)
	}
	const maxSkillSize = 256 << 10
	data, err := io.ReadAll(io.LimitReader(file, maxSkillSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSkillSize {
		return nil, fmt.Errorf("%s exceeds %d bytes", s.path, maxSkillSize)
	}
	// Eino strips the leading line prefix at the first tab.
	content := "\t" + strings.ReplaceAll(string(data), "\n", "\n\t")
	return &filesystem.FileContent{Content: content}, nil
}

func (skillFiles) LsInfo(context.Context, *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	return nil, errors.ErrUnsupported
}

func (skillFiles) GrepRaw(context.Context, *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	return nil, errors.ErrUnsupported
}

func (skillFiles) Write(context.Context, *filesystem.WriteRequest) error {
	return errors.ErrUnsupported
}

func (skillFiles) Edit(context.Context, *filesystem.EditRequest) error {
	return errors.ErrUnsupported
}
