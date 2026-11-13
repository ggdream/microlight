package chat

import (
	"context"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/skills"

	"github.com/egoist/mygo/ui"
)

type skillIndex struct {
	roots   []string
	catalog *skills.Catalog
}

func (app *chatApp) replySkillDirectory(activity timeline.Entry) string {
	if activity.ResolvedTarget != "" {
		return activity.ResolvedTarget
	}
	name := activity.SkillName()
	if name == "" {
		return ""
	}
	if app.replySkills == nil {
		workspace := ""
		if chat := app.selected(); chat != nil {
			workspace = chat.workspace
		}
		index := &skillIndex{roots: skills.Roots(app.skillsHome, workspace)}
		app.replySkills = index
		if win := app.win; win != nil {
			go func() {
				catalog := skills.Discover(context.Background(), index.roots)
				win.Update(func() {
					if app.replySkills == index {
						index.catalog = catalog
					}
				})
			}()
		}
	}
	if app.replySkills.catalog != nil {
		return app.replySkills.catalog.Directory(name)
	}
	return ""
}

func (app *chatApp) skillsSettings(c *ui.Context) {
	form := &app.extensionsForm
	var catalog *skills.Catalog
	var roots []string
	if form.skillIndex != nil {
		catalog, roots = form.skillIndex.catalog, form.skillIndex.roots
	}
	settings.SkillList(c, icons, catalog, roots, &form.skillQuery, &form.expandedSkill, app.refreshSkills)
}

func (app *chatApp) refreshSkills() {
	index := &skillIndex{roots: skills.Roots(app.skillsHome, "")}
	app.extensionsForm.skillIndex = index
	win := app.win
	go func() {
		catalog := skills.Discover(context.Background(), index.roots)
		win.Update(func() {
			if app.settingsOpen && app.extensionsForm.skillIndex == index {
				index.catalog = catalog
			}
		})
	}()
}
