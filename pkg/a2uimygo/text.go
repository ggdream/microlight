package a2uimygo

import (
	"regexp"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

var markdownHTML = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9-]*(?:[ 	][^<>]*)?/?>|<!--[\s\S]*?-->`)

// Text uses native spans, so Markdown links and images cannot initiate
// navigation or network requests from a Text component.
func (r renderer) text(c *ui.Context, node a2ui.Component, scope string) {
	source := a2ui.Text(r.property(node, scope, "text"))
	size, color := c.Theme().FontSize, r.Style.Text
	if r.surface.String(node.Props["variant"], scope) == "caption" {
		size, color = 11, r.Style.Muted
	}
	if !strings.ContainsAny(source, "*_`~[") && !strings.ContainsAny(source, "#-+>0123456789") {
		r.decorate(c, node, scope, ui.Text(c, source).FontSize(size).TextColor(color).Selectable(), source)
		return
	}
	doc := richtext.ParseMarkdown(markdownHTML.ReplaceAllStringFunc(source, func(tag string) string {
		return "\\" + tag
	}))
	var spans []ui.Span
	for i, paragraph := range doc.Paragraphs() {
		if i != 0 {
			spans = append(spans, ui.Span{Text: "\n", Size: size, Color: color})
		}
		for _, run := range paragraph.Runs {
			attrs := run.Attributes
			text := run.Text
			if attrs.Attachment != nil {
				text = attrs.Attachment.Name
			}
			font := ""
			runSize, weight := size, attrs.Weight
			if paragraph.Style.Heading > 0 {
				runSize += float32(7 - paragraph.Style.Heading)
				weight = max(weight, 650)
			}
			if attrs.Code {
				font = "monospace"
			}
			spans = append(spans, ui.Span{
				Text: text, Font: font, Size: runSize, Weight: weight, Italic: attrs.Italic,
				Strikethrough: attrs.Strikethrough, Color: color,
			})
		}
	}
	var visible strings.Builder
	for _, span := range spans {
		visible.WriteString(span.Text)
	}
	r.decorate(c, node, scope, ui.RichText(c, spans...).Label(visible.String()).Selectable(), visible.String())
}
