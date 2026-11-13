package composer

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

const skillChipURL = "microlight-skill:"
const skillChipTag = "skill-chip"

var savedSkillChip = regexp.MustCompile(`!\[\]\((?:microlight|` + storage.LegacyAgentName + `)-skill:([\p{L}\p{N}_.:\-]+)\)`)

func SkillChip(name string) richtext.Text {
	const padding = 6
	label := ui.ShapeText(name, ui.Font{Size: 12, Weight: 600}, 0)
	width, height := label.Size()
	chip := &richtext.Attachment{
		Name: name, URL: skillChipURL + name, Width: width + 2*padding, Height: height + 2,
		Draw: func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, theme.Tint, 4)
			p.Clip(r, 4, func() { p.TextLayout(label, r.X+padding, r.Y+(r.H-height)/2, theme.Accent) })
		},
	}
	return richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{
		{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Tag: skillChipTag, Attachment: chip}},
	}})
}

func skillChipName(attachment *richtext.Attachment) string {
	name, ok := strings.CutPrefix(attachment.URL, skillChipURL)
	if ok && ValidSkillName(name) {
		return name
	}
	return ""
}

func RestoreSkillChips(source string) richtext.Text {
	text := richtext.NewText(source)
	for _, match := range slices.Backward(savedSkillChip.FindAllStringSubmatchIndex(source, -1)) {
		start := utf8.RuneCountInString(source[:match[0]])
		end := start + utf8.RuneCountInString(source[match[0]:match[1]])
		text = text.Replace(start, end, SkillChip(source[match[2]:match[3]]))
	}
	return text
}

type Slash struct {
	Start, End int
	Query      string
}

func skillNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune("-_.:", r)
}

func SkillSlashAt(text string, anchor, caret int) (Slash, bool) {
	if anchor != caret {
		return Slash{}, false
	}
	runes := []rune(text)
	if caret <= 0 || caret > len(runes) {
		return Slash{}, false
	}
	start, end := caret, caret
	for start > 0 && skillNameRune(runes[start-1]) {
		start--
	}
	if start == 0 || runes[start-1] != '/' || (start > 1 && !unicode.IsSpace(runes[start-2])) {
		return Slash{}, false
	}
	for end < len(runes) && skillNameRune(runes[end]) {
		end++
	}
	if end < len(runes) && !unicode.IsSpace(runes[end]) {
		return Slash{}, false
	}
	return Slash{Start: start - 1, End: end, Query: string(runes[start:caret])}, true
}

func ValidSkillName(name string) bool {
	return strings.IndexFunc(name, func(r rune) bool { return !skillNameRune(r) }) < 0
}
