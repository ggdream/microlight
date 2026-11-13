package chat

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/internal/skills"
)

func TestSkillDiscoveryScopesAndPrecedence(t *testing.T) {
	home := t.TempDir()
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	workspace := filepath.Join(repo, "module")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	add := func(root, folder, name, body string) {
		t.Helper()
		write(filepath.Join(root, folder, "SKILL.md"), "---\nname: "+name+"\ndescription: Example\n---\n"+body)
	}
	root := func(dir string) string { return filepath.Join(dir, ".agents", "skills") }
	write(filepath.Join(repo, ".git"), "gitdir: /some/worktree")
	add(root(workspace), "group/shared", "shared", "Workspace version")
	add(root(repo), "shared", "shared", "Repo version")
	add(root(repo), "repo-skill", "repo-skill", "Repo")
	add(root(parent), "outside", "outside", "Outside")
	add(root(home), "shared", "shared", "Global version")
	add(root(home), "personal", "personal", "Global")
	add(filepath.Join(home, ".codex", "skills"), "older", "older", "Legacy")
	write(filepath.Join(root(workspace), "broken", "SKILL.md"), "No front matter")
	linked := filepath.Join(t.TempDir(), "linked")
	add(filepath.Dir(linked), filepath.Base(linked), "linked", "Linked")
	if err := os.Symlink(linked, filepath.Join(root(home), "linked")); err != nil {
		t.Fatal(err)
	}
	catalog := skills.Discover(t.Context(), skills.Roots(home, workspace))
	var names []string
	for _, entry := range catalog.Entries {
		names = append(names, entry.Name)
	}
	if want := []string{"shared", "repo-skill", "linked", "personal"}; !slices.Equal(names, want) {
		t.Fatalf("discovered %v, want %v", names, want)
	}
	if catalog.Entries[0].Root != root(workspace) || catalog.Entries[2].Root != root(home) {
		t.Fatal("skill origins should follow the discovery roots, including symlinks")
	}
	got, err := catalog.Get(t.Context(), "shared")
	if err != nil || got.Content != "Workspace version" {
		t.Fatalf("workspace skill should win: %+v, %v", got, err)
	}
	if len(catalog.Issues) != 1 {
		t.Fatalf("expected one invalid manifest, got %v", catalog.Issues)
	}
	if skills.Discover(t.Context(), skills.Roots(home, "")).Has("repo-skill") {
		t.Fatal("workspace skill leaked into a chat without a workspace")
	}
}

func TestSkillSlashReplacesOnlyTheTokenAtTheCaret(t *testing.T) {
	for _, text := range []string{"https://example.com", "path/skill", "/tmp/file"} {
		if _, ok := composer.SkillSlashAt(text, len(text), len(text)); ok {
			t.Fatalf("unexpected skill popup for %q", text)
		}
	}
	app := &chatApp{}
	app.draft.SetText("你好 /old-name 保留")
	app.draft.Document.SetSelection(6, 6)
	slash, ok := composer.SkillSlashAt(app.draft.Document.Text().String(), 6, 6)
	if !ok || slash.Query != "ol" {
		t.Fatalf("expected a partial slash query, got %+v", slash)
	}
	app.insertComposerSkill("new-skill")
	if got := app.draft.Markdown(); got != "你好 /new-skill 保留" {
		t.Fatalf("replacing a slash token changed neighboring text: %q", got)
	}
	restored := &composer.Input{}
	restored.Document.SetText(composer.RestoreSkillChips(app.draft.DraftMarkdown()))
	chip := restored.Document.Text().AttributesAt(3).Attachment
	if chip == nil || chip.Name != "new-skill" || chip.Draw == nil || restored.Markdown() != app.draft.Markdown() {
		t.Fatal("restored draft lost the visible skill label or the slash invocation")
	}
}
