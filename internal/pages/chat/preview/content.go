package preview

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/diff"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
)

func Content(c *ui.Context, state *ui.ListState, activity timeline.Entry, name string, fallback *string) {
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
		if activity.Diff != nil {
			material3.EdgeDivider(ui.Row(c).FillWidth().Shrink(0).Padding(7, 10).Gap(7).AlignItems(ui.Center), material3.BottomEdge).
				Children(func() {
					ui.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 12))
					DiffCounts(c.Key("counts"), activity.Diff)
				})
		}
		if diff := activity.Diff; diff != nil && len(diff.Rows) > 0 && diff.Added+diff.Removed > 0 {
			showFileDiff(c.Key("diff"), state, diff)
			return
		}
		content := activity.Output
		if content == nil {
			content = fallback
		}
		details := activity.Details()
		if content != nil {
			details = *content
		}
		if details == "" && content == nil {
			details = locales.PreviewUnavailable()
		}
		scroll, offset := elastic.Scroll(c.Key("content"))
		scroll.Grow(1).MinHeight(0).Children(func() {
			offset.Apply(ui.Text(c, details)).Font("monospace").FontSize(viewport.Font(c, 12)).Selectable().FillWidth().Padding(12)
		})
	})
}

func showFileDiff(c *ui.Context, state *ui.ListState, diff *diff.Lines) {
	elastic.List(c, state, len(diff.Rows), func(i int, offset elastic.Offset) {
		line := diff.Rows[i]
		if line.Omitted > 0 {
			offset.Apply(ui.Text(c, locales.DiffUnchangedLines(strconv.Itoa(line.Omitted)))).FillWidth().Padding(4, 7).
				Background(theme.Side).FontSize(viewport.Font(c, 11)).TextColor(theme.Muted)
			return
		}
		background, color := theme.White, theme.Muted
		switch line.Kind {
		case "+":
			background, color = ui.Hex("#E8F2E9"), ui.Hex("#256B35")
		case "-":
			background, color = ui.Hex("#FCEDEE"), theme.ErrorFG
		}
		offset.Apply(ui.Row(c)).FillWidth().MinHeight(22).Padding(2, 7).Gap(5).AlignItems(ui.Start).Background(background).Children(func() {
			ui.Text(c, strconv.Itoa(line.Line)).Width(37).Shrink(0).TextAlign(ui.End).
				Font("monospace").FontSize(viewport.Font(c, 11)).TextColor(color)
			ui.Text(c, line.Kind).Width(12).Shrink(0).Font("monospace").FontSize(viewport.Font(c, 12)).TextColor(color)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, strings.TrimSuffix(line.Text, "\r")).Font("monospace").FontSize(viewport.Font(c, 12)).Selectable()
				if line.NoNewline {
					ui.Text(c, locales.DiffNoNewline()).FontSize(viewport.Font(c, 11)).TextColor(theme.Muted)
				}
			})
		})
	}).Grow(1).MinHeight(0)
}
