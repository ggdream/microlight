package chat

import (
	"testing"

	"github.com/ggdream/microlight/internal/skills"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/egoist/mygo/ui"
)

func TestSkillRowsExpandIndependently(t *testing.T) {
	app := &chatApp{}
	first := skills.Entry{FrontMatter: skill.FrontMatter{Name: "first-skill", Description: "First description"}, Path: "/skills/first/SKILL.md"}
	second := skills.Entry{FrontMatter: skill.FrontMatter{Name: "second-skill", Description: "Second description"}, Path: "/skills/second/SKILL.md"}
	app.extensionsForm.skillIndex = &skillIndex{catalog: &skills.Catalog{Entries: []skills.Entry{first, second}}}
	window := ui.NewTester(app.skillsSettings, 650, 480)

	click := func(name string, offset float32) {
		t.Helper()
		row, found := window.Find(name)
		if !found {
			t.Fatalf("missing skill button %q", name)
		}
		window.ClickAt(row.X+offset, row.Y+row.H/2)
	}
	check := func(entry skills.Entry, expanded bool) {
		t.Helper()
		if _, shown := window.Find(entry.Path); shown != expanded {
			t.Errorf("skill %q expanded = %v, want %v", entry.Name, shown, expanded)
		}
	}
	click(first.Name, 23)
	check(first, true)
	check(second, false)

	secondRow, found := window.Find(second.Name)
	if !found {
		t.Fatal("missing second skill button")
	}
	click(second.Name, secondRow.W-18)
	check(first, false)
	check(second, true)

	click(second.Name, 60)
	check(second, false)
}
