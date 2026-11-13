package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	project "github.com/ggdream/microlight/internal/workspace"

	"github.com/cloudwego/eino/adk/middlewares/skill"
)

func Roots(home, workspace string) []string {
	var roots []string
	for _, dir := range project.Directories(workspace) {
		roots = append(roots, filepath.Join(dir, ".agents", "skills"))
	}
	if home != "" {
		root := filepath.Join(home, ".agents", "skills")
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	return roots
}

type Entry struct {
	skill.FrontMatter
	Path    string
	Root    string
	backend skill.Backend
}

type Catalog struct {
	Entries []Entry
	index   map[string]int
	Issues  []error
}

func Discover(ctx context.Context, roots []string) *Catalog {
	catalog := &Catalog{index: make(map[string]int)}
	visited := make(map[string]bool)
	for _, root := range roots {
		catalog.scan(ctx, root, root, 0, visited)
	}
	return catalog
}

func (c *Catalog) scan(ctx context.Context, root, dir string, depth int, visited map[string]bool) {
	// Grouping folders are supported up to six levels beneath a source.
	if ctx.Err() != nil || depth > 6 {
		return
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		c.Issues = append(c.Issues, fmt.Errorf("%s: %w", dir, err))
		return
	}
	if visited[resolved] {
		return
	}
	visited[resolved] = true
	files, err := os.ReadDir(resolved)
	if err != nil {
		c.Issues = append(c.Issues, fmt.Errorf("%s: %w", dir, err))
		return
	}
	for _, file := range files {
		if depth > 0 && file.Name() == "SKILL.md" {
			c.add(ctx, root, filepath.Join(resolved, file.Name()))
			return
		}
	}
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		path := filepath.Join(resolved, name)
		if file.IsDir() {
			c.scan(ctx, root, path, depth+1, visited)
		} else if file.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				c.scan(ctx, root, path, depth+1, visited)
			}
		}
	}
}

func (c *Catalog) add(ctx context.Context, root, path string) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		c.Issues = append(c.Issues, fmt.Errorf("%s: invalid skill manifest", path))
		return
	}
	dir := filepath.Dir(path)
	backend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: skillFiles{dir: dir, path: path}, BaseDir: dir,
	})
	if err != nil {
		c.Issues = append(c.Issues, fmt.Errorf("%s: %w", path, err))
		return
	}
	entries, err := backend.List(ctx)
	if err != nil {
		c.Issues = append(c.Issues, fmt.Errorf("%s: %w", path, err))
		return
	}
	if len(entries) == 0 || strings.TrimSpace(entries[0].Name) == "" || strings.TrimSpace(entries[0].Description) == "" {
		c.Issues = append(c.Issues, fmt.Errorf("%s: missing skill name or description", path))
		return
	}
	meta := entries[0]
	if _, exists := c.index[meta.Name]; exists {
		return
	}
	c.index[meta.Name] = len(c.Entries)
	c.Entries = append(c.Entries, Entry{FrontMatter: meta, Path: path, Root: root, backend: backend})
}

func (c *Catalog) List(ctx context.Context) ([]skill.FrontMatter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries := make([]skill.FrontMatter, 0, len(c.Entries))
	for _, entry := range c.Entries {
		entries = append(entries, entry.FrontMatter)
	}
	return entries, nil
}

func (c *Catalog) Get(ctx context.Context, name string) (skill.Skill, error) {
	i, ok := c.index[name]
	if !ok {
		return skill.Skill{}, fmt.Errorf("skill not found: %s", name)
	}
	return c.Entries[i].backend.Get(ctx, name)
}

func (c *Catalog) Directory(name string) string {
	if i, ok := c.index[name]; ok {
		return filepath.Dir(c.Entries[i].Path)
	}
	return ""
}

func (c *Catalog) Has(name string) bool {
	_, ok := c.index[name]
	return ok
}
