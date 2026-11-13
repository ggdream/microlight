package chat

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/internal/skills"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

type composerSkillPicker struct {
	open, manual, focusEditor bool
	blockedSlash              int
	workspace                 string
	search, lastQuery         string
	index                     *skillIndex
	cancelScan                context.CancelFunc
	highlight                 int
	list                      ui.ListState
}

func (app *chatApp) resetComposerSkills() {
	if cancel := app.skillPicker.cancelScan; cancel != nil {
		cancel()
	}
	app.skillPicker = composerSkillPicker{}
}

func (app *chatApp) openComposerSkills(workspace string, manual bool) {
	app.resetComposerSkills()
	picker := &app.skillPicker
	picker.open, picker.manual, picker.workspace = true, manual, workspace
	index := &skillIndex{roots: skills.Roots(app.skillsHome, workspace)}
	picker.index = index
	if app.win == nil {
		index.catalog = skills.Discover(context.Background(), index.roots)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	picker.cancelScan = cancel
	go func() {
		catalog := skills.Discover(ctx, index.roots)
		if ctx.Err() == nil {
			app.win.Update(func() {
				if app.skillPicker.index == index {
					index.catalog = catalog
				}
			})
		}
	}()
}

type composerSkillRow struct {
	title string
	entry *skills.Entry
}

func (picker *composerSkillPicker) moveHighlight(rows []composerSkillRow, step int) {
	for i := picker.highlight + step; i > 0 && i < len(rows); i += step {
		if rows[i].entry != nil {
			picker.highlight = i
			picker.list.ScrollIntoView(i)
			return
		}
	}
}

func composerSkillRows(index *skillIndex, globalRoot, query string) []composerSkillRow {
	if index == nil || index.catalog == nil {
		return nil
	}
	var workspace, global []*skills.Entry
	query = strings.ToLower(strings.TrimSpace(query))
	for i := range index.catalog.Entries {
		entry := &index.catalog.Entries[i]
		if !composer.ValidSkillName(entry.Name) ||
			(!strings.Contains(strings.ToLower(entry.Name), query) && !strings.Contains(strings.ToLower(entry.Description), query)) {
			continue
		}
		if entry.Root == globalRoot {
			global = append(global, entry)
		} else {
			workspace = append(workspace, entry)
		}
	}
	var rows []composerSkillRow
	for _, group := range []struct {
		title   string
		entries []*skills.Entry
	}{{locales.WorkspaceSkills(), workspace}, {locales.GlobalSkills(), global}} {
		if len(group.entries) == 0 {
			continue
		}
		slices.SortFunc(group.entries, func(a, b *skills.Entry) int { return strings.Compare(a.Name, b.Name) })
		rows = append(rows, composerSkillRow{title: group.title})
		for _, entry := range group.entries {
			rows = append(rows, composerSkillRow{entry: entry})
		}
	}
	return rows
}

func (app *chatApp) composerSkills(c *ui.Context, editor ui.Element, workspace string) bool {
	picker := &app.skillPicker
	if picker.focusEditor {
		editor.Focus()
		picker.focusEditor = false
	}
	anchor, caret := app.draft.Document.Selection()
	slash, hasSlash := composer.SkillSlashAt(app.draft.Document.Text().String(), anchor, caret)
	if !hasSlash {
		picker.blockedSlash = 0
	}
	if picker.open && (!picker.manual && !hasSlash || picker.workspace != workspace) {
		app.resetComposerSkills()
	}
	if !picker.open && editor.Focused() && hasSlash && picker.blockedSlash != slash.Start+1 {
		app.openComposerSkills(workspace, false)
	}
	if !picker.open {
		return false
	}
	query := slash.Query
	if picker.manual {
		query = picker.search
	}
	rows := composerSkillRows(picker.index, filepath.Join(app.skillsHome, ".agents", "skills"), query)
	if query != picker.lastQuery || picker.highlight >= len(rows) {
		picker.highlight, picker.lastQuery = 0, query
		picker.list.ScrollTo(0, ui.Start)
	}
	if len(rows) != 0 && picker.highlight == 0 {
		picker.highlight = 1
	}
	ui.PopoverBase(c, editor, &picker.open, func(panel ui.Element) {
		panel.AttachTo(editor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).
			Width(390).Padding(6).Gap(4).Radius(11).Background(white).Border(1, border).
			Shadow(0, 5, 18, 0, ui.RGBA(29, 49, 38, .12)).Label(locales.ChooseSkill())
		if picker.manual {
			label := locales.SearchSkills()
			textInput(c.Key("search"), &picker.search).Label(label).Placeholder(label).
				FillWidth().Height(32).PaddingX(8).Border(1, border).Radius(6).AutoFocus().
				OnTextKey(func(key ui.TextKey) bool {
					if key.Mods != 0 {
						return false
					}
					switch key.Key {
					case ui.KeyDown:
						picker.moveHighlight(rows, 1)
					case ui.KeyUp:
						picker.moveHighlight(rows, -1)
					case ui.KeyEnter:
						if len(rows) != 0 {
							app.insertComposerSkill(rows[picker.highlight].entry.Name)
						}
					default:
						return false
					}
					return true
				})
		}
		if len(rows) == 0 {
			label := locales.NoMatchingSkills()
			if picker.index.catalog == nil {
				label = locales.ScanningSkills()
			} else if strings.TrimSpace(query) == "" {
				label = locales.NoSkillsFound()
			}
			ui.Text(c, label).FontSize(12).TextColor(muted).Padding(7, 8)
			// Keep Enter from submitting an unfinished slash token.
			if !picker.manual {
				editor.Shortcut(0, ui.KeyEnter)
			}
			return
		}
		if !picker.manual {
			if editor.Shortcut(0, ui.KeyDown) {
				picker.moveHighlight(rows, 1)
			}
			if editor.Shortcut(0, ui.KeyUp) {
				picker.moveHighlight(rows, -1)
			}
			if editor.Shortcut(0, ui.KeyEnter) {
				app.insertComposerSkill(rows[picker.highlight].entry.Name)
				return
			}
		}
		height := float32(0)
		for _, row := range rows {
			if row.entry == nil {
				height += 24
			} else {
				height += 43
			}
		}
		ui.List(c.Key("skills"), &picker.list, len(rows), func(i int) {
			row := rows[i]
			if row.entry == nil {
				ui.Row(c).Height(24).AlignItems(ui.Center).PaddingX(8).Children(func() {
					ui.Text(c, row.title).FontSize(11).FontWeight(620).TextColor(muted)
				})
				return
			}
			entry := row.entry
			button := pointerButton(c, false).Label(entry.Name).Tooltip(entry.Description).
				FillWidth().Height(43).PaddingX(8).Gap(8).Radius(6).AlignItems(ui.Center)
			if i == picker.highlight || button.Hovered() {
				button.Background(tint)
			}
			button.Children(func() {
				icon(c, "document_text", muted, 16)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, entry.Name).SingleLine().Ellipsis("…").Clip().FontSize(13).FontWeight(620)
					ui.Text(c, strings.Join(strings.Fields(entry.Description), " ")).SingleLine().
						Ellipsis("…").Clip().FontSize(11).TextColor(muted)
				})
			}).OnClick(func() { app.insertComposerSkill(entry.Name) })
		}).FillWidth().Height(min(264, height))
	})
	if !picker.open && picker.index != nil {
		app.resetComposerSkills()
		if hasSlash {
			picker.blockedSlash = slash.Start + 1
		}
	}
	return true
}

func (app *chatApp) insertComposerSkill(name string) {
	doc := &app.draft.Document
	text := doc.Text()
	anchor, caret := doc.Selection()
	from, to := min(anchor, caret), max(anchor, caret)
	if slash, ok := composer.SkillSlashAt(text.String(), anchor, caret); ok {
		from, to = slash.Start, slash.End
	}
	prefix := ""
	if from > 0 {
		left, _ := utf8.DecodeRuneInString(text.Slice(from-1, from).String())
		if !unicode.IsSpace(left) {
			prefix = " "
		}
	}
	if to < text.Len() && text.Slice(to, to+1).String() == " " {
		to++
	}
	doc.Edit(func(edit *richtext.Edit) {
		token := richtext.NewText(prefix + " ")
		token = token.Replace(len(prefix), len(prefix), composer.SkillChip(name))
		edit.Replace(from, to, token)
		end := from + token.Len()
		edit.Select(end, end)
	})
	app.resetComposerSkills()
	app.skillPicker.focusEditor = true
}
