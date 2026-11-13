package composer

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/egoist/mygo/ui/richtext"
)

const composerFontSize = 14

type Input struct {
	Document         richtext.Document
	scanned          uint64
	lastConverted    string
	editingMath      bool
	filePasteVersion *uint64
}

type composerFormula struct {
	source  string
	display bool
}

func (input *Input) SetText(source string) {
	input.Document.SetText(richtext.NewText(source))
	input.lastConverted = ""
	input.editingMath = false
	input.scanned = 0
	input.filePasteVersion = nil
}

func (input *Input) DiscardFilePasteText() {
	if input.filePasteVersion == nil {
		return
	}
	version := *input.filePasteVersion
	input.filePasteVersion = nil
	// Richtext also accepts the plain-text representation of a native file list.
	// Undo only that one paste, leaving the draft and its selection intact.
	if input.Document.Version() == version+1 {
		input.Document.Undo()
		input.Document.BreakUndoCoalescing()
	}
}

func (input *Input) View(c *ui.Context, label, placeholder string, height float32, md *markdown.Options) ui.Element {
	font := ui.Font{Size: composerFontSize, Family: c.Theme().Font}
	editor := richtext.EditorBase(c, &input.Document, richtext.EditorOptions{
		Label: label, Placeholder: placeholder, Font: font,
		Theme: &richtext.Theme{Caret: theme.CaretColor},
		TagStyle: func(tag string, span *ui.Span) {
			if tag == skillChipTag {
				// Inline attachments start entirely above the baseline.
				span.BaselineShift = -font.Metrics().Descent
			}
		},
	}).FillWidth().Height(height).AutoFocus().FocusRingColor(theme.Accent).FocusRingOffset(3)

	version := input.Document.Version()
	if editor.Shortcut(ui.Alt, ui.KeyEnter) || editor.DoubleClicked() {
		input.editFormula()
	}
	if input.scanned != version || input.editingMath {
		input.restorePastedSkillChips()
		input.renderFormatting(md)
		input.scanned = input.Document.Version()
	}
	if input.Document.Version() != version {
		c.AnimationFrame()
	}
	return editor
}

func (input *Input) restorePastedSkillChips() {
	text := input.Document.Text()
	if !strings.ContainsRune(text.String(), richtext.ObjectReplacement) {
		return
	}
	pos := 0
	for _, paragraph := range text.Paragraphs() {
		for _, run := range paragraph.Runs {
			if attachment := run.Attributes.Attachment; attachment != nil && attachment.Draw == nil {
				if name := skillChipName(attachment); name != "" {
					at := pos
					input.Document.Edit(func(edit *richtext.Edit) { edit.Replace(at, at+1, SkillChip(name)) })
				}
			}
			pos += utf8.RuneCountInString(run.Text)
		}
		pos++
	}
}

func (input *Input) editFormula() {
	t := input.Document.Text()
	a, z := input.Document.Selection()
	start := min(a, z)
	if a == z && start == t.Len() {
		start--
	}
	for _, pos := range []int{start, start - 1} {
		if pos < 0 || pos >= t.Len() {
			continue
		}
		attrs := t.AttributesAt(pos)
		if attrs.Attachment == nil {
			continue
		}
		formula, ok := attrs.Attachment.Data.(composerFormula)
		if !ok {
			continue
		}
		attrs.Attachment = nil
		raw := formula.source
		delimiter := 2
		if strings.HasPrefix(raw, "$") && !strings.HasPrefix(raw, "$$") {
			delimiter = 1
		}
		input.Document.Edit(func(edit *richtext.Edit) {
			edit.ReplaceString(pos, pos+1, raw, attrs)
			edit.Select(pos+delimiter, pos+utf8.RuneCountInString(raw)-delimiter)
		})
		input.editingMath = true
		input.lastConverted = ""
		return
	}
}

func (input *Input) renderFormatting(md *markdown.Options) {
	t := input.Document.Text()
	raw := t.String()
	if md == nil || raw == input.lastConverted || !strings.ContainsAny(raw, "$\\*_~`[") {
		return
	}
	parsed := parser.New(md.Extensions)
	parsed.Set(raw)
	a, z := input.Document.Selection()

	type replacement struct {
		start, end int
		text       richtext.Text
	}
	var replacements []replacement
	stillEditing := false
	add := func(start, end int, source string, display bool) {
		if start < 0 || end > len(raw) || start >= end {
			return
		}
		rawFormula := raw[start:end]
		if strings.ContainsRune(rawFormula, richtext.ObjectReplacement) {
			return
		}
		from := utf8.RuneCountInString(raw[:start])
		to := from + utf8.RuneCountInString(rawFormula)
		if input.editingMath && min(a, z) < to && max(a, z) > from {
			stillEditing = true
			return
		}
		attrs := t.AttributesAt(from)
		if attrs.Code || t.ParagraphStyleAt(from).CodeBlock {
			return
		}
		if md.InlineMath == nil {
			return
		}
		source = strings.TrimSpace(source)
		if display {
			source = `\displaystyle ` + source
		}
		draw, ok := md.InlineMath(source, composerFontSize)
		if !ok {
			return
		}
		attrs.Attachment = &richtext.Attachment{
			Name: rawFormula, Data: composerFormula{source: rawFormula, display: display},
			Width: draw.Width, Height: draw.Height + draw.Depth, Draw: draw.Draw,
		}
		formatted := richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{
			Text: string(richtext.ObjectReplacement), Attributes: attrs,
		}}})
		replacements = append(replacements, replacement{from, to, formatted})
	}
	for i := 1; i < parsed.Len(); i++ {
		block := parsed.Block(i)
		if block.Kind == parser.MathBlock && !block.Open {
			end := block.Start + len(strings.TrimRight(raw[block.Start:block.End], " \t\r\n"))
			add(block.Start, end, parsed.Lines(i), true)
			continue
		}
		tokens := parsed.Inlines(i)
		for j := 0; j < len(tokens); j++ {
			token := tokens[j]
			if token.Kind == parser.Math {
				add(token.Start, token.End, token.Text, token.Display)
				continue
			}
			switch token.Kind {
			case parser.Emphasis, parser.Strong, parser.Strikethrough, parser.Code, parser.Link:
			default:
				continue
			}
			if token.Auto || token.Start < 0 || token.End > len(raw) || token.Start >= token.End {
				continue
			}
			source := raw[token.Start:token.End]
			if strings.ContainsAny(source, "$\\\n"+string(richtext.ObjectReplacement)) {
				continue
			}
			from := utf8.RuneCountInString(raw[:token.Start])
			to := from + utf8.RuneCountInString(source)
			if t.ParagraphStyleAt(from).CodeBlock || t.AttributesAt(from) != (richtext.Attributes{}) {
				continue
			}
			formatted := richtext.ParseMarkdown(source)
			if formatted.Len() != 0 && formatted.String() != source {
				replacements = append(replacements, replacement{from, to, formatted})
				j = token.Next - 1
			}
		}
	}
	input.editingMath = stillEditing
	if len(replacements) == 0 {
		return
	}
	input.lastConverted = raw
	input.Document.Edit(func(edit *richtext.Edit) {
		for i := len(replacements) - 1; i >= 0; i-- {
			part := replacements[i]
			edit.Replace(part.start, part.end, part.text)
		}
	})
}

// Markdown restores the source of inline attachments before sending.
func (input *Input) DraftMarkdown() string {
	return input.markdown(true)
}

// MarkFilePaste records the editor state before the native file paste is handled.
func (input *Input) MarkFilePaste() {
	version := input.Document.Version()
	input.Document.BreakUndoCoalescing()
	input.filePasteVersion = &version
}

func (input *Input) Markdown() string {
	return input.markdown(false)
}

func (input *Input) markdown(saveDraft bool) string {
	t := input.Document.Text()
	var paragraphs []richtext.Paragraph
	var plain, plainGroup strings.Builder
	var substitutions []string
	prefix := "QMicrolightFormula"
	for strings.Contains(t.String(), prefix) {
		prefix += "Q"
	}
	hasStyles := false
	replaceWithMarker := func(source string) string {
		marker := prefix + strconv.Itoa(len(substitutions)/2) + "Q"
		substitutions = append(substitutions, marker, source)
		return marker
	}
	pending := false
	flushPlainGroup := func() {
		if !pending {
			return
		}
		paragraphs = append(paragraphs, richtext.Paragraph{
			Runs: []richtext.Run{{Text: replaceWithMarker(plainGroup.String())}},
		})
		plainGroup.Reset()
		pending = false
	}
	first := true
	for _, paragraph := range t.Paragraphs() {
		if !first {
			plain.WriteByte('\n')
		}
		first = false
		styled := paragraph.Style != (richtext.ParagraphStyle{})
		var line strings.Builder
		for i, run := range paragraph.Runs {
			attrs := run.Attributes
			if attrs.Attachment != nil {
				if name := skillChipName(attrs.Attachment); name != "" {
					source := "/" + name
					if saveDraft {
						source = "![](" + skillChipURL + name + ")"
					}
					line.WriteString(source)
					paragraph.Runs[i] = richtext.Run{Text: replaceWithMarker(source)}
					continue
				}
				if formula, ok := attrs.Attachment.Data.(composerFormula); ok {
					source := formula.source
					if formula.display && strings.ContainsRune(source, '\n') {
						source = "\n\n" + source + "\n\n"
					}
					line.WriteString(formula.source)
					attrs.Attachment = nil
					styled = styled || attrs != (richtext.Attributes{})
					paragraph.Runs[i] = richtext.Run{Text: replaceWithMarker(source), Attributes: attrs}
					continue
				}
			}
			styled = styled || attrs != (richtext.Attributes{})
			line.WriteString(run.Text)
		}
		plain.WriteString(line.String())
		if styled {
			hasStyles = true
			flushPlainGroup()
			paragraphs = append(paragraphs, paragraph)
		} else {
			// Untouched source such as tables and fenced code needs its original line breaks.
			if pending {
				plainGroup.WriteByte('\n')
			}
			plainGroup.WriteString(line.String())
			pending = true
		}
	}
	if !hasStyles {
		return plain.String()
	}
	flushPlainGroup()
	source := richtext.TextOf(paragraphs...).Markdown()
	return strings.NewReplacer(substitutions...).Replace(source)
}
