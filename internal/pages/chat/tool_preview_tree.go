package chat

import (
	"cmp"
	"path/filepath"
	"slices"

	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/chat/preview"
)

func roundChangedFiles(entry *turn, workspace string) []preview.Change {
	if entry == nil {
		return nil
	}
	byPath := make(map[string]int)
	for index, activity := range entry.Activity {
		if activity.Tool != localtools.Write || (activity.Diff == nil && activity.Output == nil) ||
			(activity.Diff != nil && activity.Diff.Added+activity.Diff.Removed == 0) {
			continue
		}
		path := activity.DisplayTarget()
		if path == "" {
			continue
		}
		if workspace != "" && !filepath.IsAbs(path) {
			path = filepath.Join(workspace, path)
		}
		byPath[filepath.Clean(path)] = index
	}
	files := make([]preview.Change, 0, len(byPath))
	for path, index := range byPath {
		files = append(files, preview.Change{Path: path, Index: index, Icon: previewIcon(entry.Activity[index]), Diff: entry.Activity[index].Diff})
	}
	slices.SortFunc(files, func(a, b preview.Change) int { return cmp.Compare(a.Path, b.Path) })
	return files
}
