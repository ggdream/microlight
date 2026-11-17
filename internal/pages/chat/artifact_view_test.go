package chat

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/diff"
)

func TestFileDeliverablesSummary(t *testing.T) {
	workspace := t.TempDir()
	entry := &turn{State: "done"}
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		entry.Activity = append(entry.Activity, timeline.Entry{
			Tool: localtools.Write, ResolvedTarget: filepath.Join(workspace, "src", name),
			Diff: diff.Compare("previous", "updated"),
		})
	}
	app := &chatApp{}
	window := ui.NewTester(func(c *ui.Context) { app.replyDeliverables(c, entry, workspace) }, 550, 370)
	for _, label := range []string{locales.ArtifactFilesEdited(4), "+4", "-4", "src/a.go", "src/c.go"} {
		if _, shown := window.Find(label); !shown {
			t.Fatalf("missing %q from the summary: %v", label, window.Texts())
		}
	}
	if _, shown := window.Find("src/d.go"); shown {
		t.Fatal("the fourth file should start folded")
	}
	if err := window.Click(locales.ArtifactShowMore(1)); err != nil {
		t.Fatal(err)
	}
	if err := window.Click("src/d.go"); err != nil {
		t.Fatal(err)
	}
	if !app.toolPanel.visible || app.toolPanel.active == nil || app.toolPanel.active.index != 3 {
		t.Fatal("the file row should open that file's diff")
	}
	if err := window.Click(locales.ArtifactViewChanges()); err != nil {
		t.Fatal(err)
	}
	if app.toolPanel.round != entry || app.toolPanel.active != nil {
		t.Fatal("the summary action should open the reply's full change tree")
	}
}
