package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/chat/preview"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/ggdream/microlight/pkg/elastic"
)

func replyReasoning(entry *turn) (hasReasoning, open bool) {
	hasBody := strings.TrimSpace(entry.Content) != "" || len(entry.Interface) > 0
	compressing := entry.State == "pending" && entry.compressing
	hasReasoning = compressing || slices.ContainsFunc(entry.Activity, func(a timeline.Entry) bool { return a.Tool != timeline.AskUserTool }) || strings.TrimSpace(entry.processPreview) != "" || slices.ContainsFunc(entry.Clarifications, func(q clarification) bool {
		return q.Answer != ""
	})
	open = (entry.State == "pending" || entry.State == "waiting") && !hasBody
	if entry.reasoningOpen != nil {
		open = *entry.reasoningOpen
	}
	return hasReasoning, open
}

func (entry *turn) visibleTool(activity timeline.Entry) bool {
	return !activity.Pending || !localtools.TracksToolStart(activity.Tool) || entry.activeTools[activity.CallID]
}

func replyRunningStatus(entry *turn) string {
	if slices.ContainsFunc(entry.Activity, func(activity timeline.Entry) bool {
		return activity.Tool != timeline.AskUserTool && activity.Pending && entry.visibleTool(activity)
	}) {
		return ""
	}
	if entry.compressing || len(entry.Interface) > 0 || len(entry.interfacePreview) > 0 || strings.TrimSpace(entry.processPreview) != "" || strings.TrimSpace(entry.Content) != "" {
		return ""
	}
	return locales.Thinking()
}

func shimmerReplyText(c *ui.Context, element ui.Element) {
	if c.Preferences().ReduceMotion {
		return
	}
	const period = 2200 * time.Millisecond
	const width = float32(.22)
	phase := float32(c.Now().UnixNano()%int64(period)) / float32(period)
	center := phase*1.55 - .275
	maskColor := func(at float32) ui.Color {
		distance := max(at-center, center-at)
		return ui.RGB(0, 0, 0).Alpha(1 - .42*max(0, 1-distance/width))
	}
	stops := []ui.GradientStop{{Color: maskColor(0), At: 0}}
	for _, at := range []float32{center - width, center, center + width} {
		if at > 0 && at < 1 {
			stops = append(stops, ui.GradientStop{Color: maskColor(at), At: at})
		}
	}
	stops = append(stops, ui.GradientStop{Color: maskColor(1), At: 1})
	element.MaskLinearGradient(ui.LinearGradient{Angle: 90, Stops: stops})
	c.After(time.Second / 30)
}

func replyProgress(c *ui.Context, entry *turn, hasReasoning, open bool) {
	label := replyStatus(entry, c.Now())
	if label == "" && !hasReasoning {
		return
	}
	if label == "" && hasReasoning {
		label = locales.Reasoning()
	}
	if entry.State == "pending" && !entry.StartedAt.IsZero() {
		elapsed := max(0, c.Now().Sub(entry.StartedAt))
		c.After(time.Second - elapsed%time.Second)
	}

	header := ui.Row(c).FillWidth().AlignItems(ui.Center)
	if entry.State == "done" && hasReasoning {
		header.Padding(0, 0, 5, 0).Margin(0, 0, 5, 0).
			BorderWidth(0, 0, 1, 0).BorderColor(border)
	}
	header.Children(func() {
		switch {
		case hasReasoning:
			accessLabel := locales.Reasoning()
			if label != accessLabel {
				accessLabel = locales.ReasoningAccessibility(label)
			}
			button := pointerButton(c.Key("reasoning"), false).Label(accessLabel).
				Expanded(open).Justify(ui.Start).AlignItems(ui.Center).Gap(5).
				Padding(3, 5, 3, 0).Radius(5)
			if button.Hovered() {
				button.Background(side)
			}
			button.Children(func() {
				ui.Text(c, label).FontSize(windowLayout(c).Text(windowLayout(c).Density(12, 14))).TextColor(muted)
				chevron := ui.Icon(c, icons.SVG("ios_chevron_right")).Size(16, 16).TextColor(muted)
				if open {
					chevron.Rotate(90)
				}
			}).OnClick(func() {
				show := !open
				entry.reasoningOpen = &show
			})
		case label != "":
			ui.Text(c, label).FontSize(windowLayout(c).Text(windowLayout(c).Density(12, 14))).TextColor(muted).Padding(3, 0)
		}
	})
}

func (app *chatApp) replyBody(c *ui.Context, chat *conversation, entry *turn, interfaces *interfaceProjection, showReasoning bool) {
	hasBody := strings.TrimSpace(entry.Content) != ""
	rows := interfaces.activeRows(entry)
	if !showReasoning && !hasBody && len(rows) == 0 {
		return
	}
	ui.Column(c.Key("reply-body")).FillWidth().Gap(5).Children(func() {
		if showReasoning {
			app.reasoningSegments(c, entry)
			if entry.State == "pending" && entry.compressing {
				ui.Row(c.Key("context-compression")).AlignItems(ui.Center).Gap(8).Padding(4, 0).Children(func() {
					icon(c, "document_text", muted, 16)
					ui.Text(c, locales.Compressing()).FontSize(viewport.Font(c, 12)).TextColor(muted)
				})
			}
		}
		if len(rows) > 0 {
			app.renderInterfaceAnswer(c.Key("interface-answer"), chat, entry, interfaces, rows)
		} else if hasBody {
			app.replyText(c.Key("answer"), entry.Content, entry.State == "pending")
		}
	})
}

func (app *chatApp) replyText(c *ui.Context, text string, streaming bool) {
	style := markdown.DefaultTheme(c.Theme())
	if windowLayout(c).Compact() {
		style.LineHeight = 1.4
	}
	if app.md != nil {
		app.md.Theme = style
	}
	if streaming {
		markdown.Stream(c, text, app.md)
	} else {
		markdown.View(c, text, app.md)
	}
}

func (app *chatApp) reasoningSegments(c *ui.Context, entry *turn) {
	type answerKey int
	approvals := entry.linkedApprovals()
	start := 0
	showActivity := func(end int) {
		for i, activity := range entry.Activity[start:end] {
			if activity.Tool == "" {
				app.replyText(c.Key(start+i), activity.Text, false)
				continue
			}
			if approval, ok := approvals[activity.CallID]; ok {
				question := &entry.Clarifications[approval]
				answeredClarification(c.Key(answerKey(approval)), question)
				if question.Answer == localtools.Deny {
					continue
				}
			}
			if activity.Tool != timeline.AskUserTool && entry.visibleTool(activity) {
				app.toolActivityRow(c.Key(start+i), entry, start+i)
			}
		}
		start = end
	}
	for i := range entry.Clarifications {
		question := &entry.Clarifications[i]
		if question.Answer == "" {
			continue
		}
		if linked, ok := approvals[entry.approvalCallID(question)]; ok && linked == i {
			continue
		}
		end := len(entry.Activity)
		if offset := question.ActivityOffset; offset != nil {
			end = min(max(*offset, start), end)
		}
		showActivity(end)
		answeredClarification(c.Key(answerKey(i)), question)
	}
	showActivity(len(entry.Activity))
	if text := strings.TrimSpace(entry.processPreview); text != "" && len(entry.interfacePreview) == 0 {
		app.replyText(c.Key("stream"), text, true)
	}
}

func (app *chatApp) toolActivityRow(c *ui.Context, entry *turn, index int) {
	activity := entry.Activity[index]
	target := activity.ShortTarget()
	command := activity.RunsCommand()
	fileTarget := target != "" && (activity.Tool == localtools.Read || activity.Tool == localtools.List || activity.Tool == localtools.Write)
	expandable := command || (!fileTarget && activity.Tool != timeline.AskUserTool &&
		activity.Tool != localtools.WebSearch && activity.Tool != localtools.BrowserUse &&
		(activity.Arguments != "" || activity.Output != nil))
	row := ui.Column(c).FillWidth().Gap(4)
	open := ui.Local(row, "tool-details-open", func() bool { return false })
	row.Children(func() {
		var line ui.Element
		if expandable {
			label := locales.ToolDetails()
			if command {
				label = locales.ToolArguments()
			}
			line = pointerButton(c.Key("toggle"), false).Label(label).Expanded(*open).
				OnClick(func() { *open = !*open })
		} else {
			line = ui.Row(c)
		}
		line.FillWidth().Justify(ui.Start).AlignItems(ui.Center).Gap(8).Padding(4, 0)
		bodyStyle := markdown.DefaultTheme(c.Theme())
		hovered := expandable && line.Hovered()
		color := muted
		if hovered {
			color = bodyStyle.Text
		}
		line.Children(func() {
			labelGroup := ui.Row(c.Key("tool-label")).MinWidth(0).Gap(8).AlignItems(ui.Center)
			if activity.Tool == timeline.SkillTool {
				if directory := app.replySkillDirectory(activity); directory != "" {
					labelGroup.Tooltip(directory)
				}
			}
			if entry.State == "pending" && activity.Pending {
				shimmerReplyText(c, labelGroup)
			}
			labelGroup.Children(func() {
				icon(c, activity.Icon(), color, 16)
				label := activity.Label(target)
				if fileTarget {
					label = activity.Label("")
				}
				ui.Text(c, label).MinWidth(0).SingleLine().Ellipsis("…").FontSize(bodyStyle.FontSize).TextColor(color)
				if fileTarget {
					path := activity.DisplayTarget()
					if !filepath.IsAbs(path) {
						if chat := app.selected(); chat != nil && filepath.IsAbs(chat.workspace) {
							path = filepath.Join(chat.workspace, path)
						} else {
							path = ""
						}
					}
					ui.Themed(c, theme.PopupTheme(c)).Background(ui.Transparent).MinWidth(0).
						AlignItems(ui.Start).Children(func() {
						button := pointerButton(c.Key("target"), false).Label(activity.Label(activity.ShortTarget())).
							MinWidth(0).MaxWidthPercent(100).Tooltip(path)
						linkColor := muted
						highlight := button.Hovered() || button.FocusVisible()
						if highlight {
							linkColor = bodyStyle.Text
						}
						button.Children(func() {
							link := ui.Text(c, target).MinWidth(0).SingleLine().Ellipsis("…").
								UnderlineOffset(2).FontSize(bodyStyle.FontSize).TextColor(linkColor)
							if highlight {
								link.Underline()
							} else {
								link.DottedUnderline()
							}
						}).OnClick(func() { app.openToolPreview(entry, index) })
						items := []theme.PopupItem{
							{Label: locales.OpenPreview(), Icon: icons.Menu("panel_left_expand"), OnSelect: func() {
								app.openToolPreview(entry, index)
							}},
						}
						if path != "" && activity.Tool != localtools.List {
							quickLook := func() { app.showPreviewError(desktop.PreviewFile(path)) }
							if button.Shortcut(0, ui.KeySpace) {
								quickLook()
							}
							items = append(items, theme.PopupItem{Label: desktop.PreviewLabel(), Icon: icons.Menu("open"), OnSelect: quickLook})
						}
						if full := activity.DisplayTarget(); full != "" {
							items = append(items, theme.PopupItem{Label: locales.CopyTarget(), Icon: icons.Menu("copy"), OnSelect: func() {
								c.WriteClipboard(full)
							}})
						}
						theme.PopupContextMenu(button, items...)
					})
				}
			})
			if diff := activity.Diff; diff != nil {
				preview.DiffCounts(c.Key("diff"), diff)
			}
			if expandable {
				replyActivityChevron(c, !command || hovered || line.FocusVisible(), *open, color, 16)
			}
		})
		if expandable && *open {
			if command {
				shellCard(c.Key("details"), activity)
			} else {
				toolActivityCard(c.Key("details"), activity)
			}
		}
	})
}

func replyActivityChevron(c *ui.Context, visible, open bool, color ui.Color, height float32) {
	ui.Box(c.Key("spacer")).Grow(1)
	ui.Box(c.Key("arrow")).Size(23, height).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		if visible || open {
			chevron := ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).TextColor(color)
			if open {
				chevron.Rotate(90)
			}
		}
	})
}

func answeredClarification(c *ui.Context, question *clarification) {
	title := locales.AskUserTool()
	if question.Approval != nil {
		title = locales.ApprovalRecord()
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		bodyStyle := markdown.DefaultTheme(c.Theme())
		button := pointerButton(c.Key("toggle"), false).Label(title).Expanded(question.answerOpen).
			FillWidth().Justify(ui.Start).AlignItems(ui.Center).Gap(8).Padding(4, 0)
		color := muted
		if button.Hovered() {
			color = bodyStyle.Text
		}
		button.Children(func() {
			icon(c, "question", color, 16)
			ui.Text(c, title).MinWidth(0).SingleLine().Ellipsis("…").FontSize(bodyStyle.FontSize).TextColor(color)
			replyActivityChevron(c, button.Hovered() || button.FocusVisible(), question.answerOpen, bodyStyle.Text, 23)
		}).OnClick(func() {
			question.answerOpen = !question.answerOpen
		})
		if question.answerOpen {
			ui.Column(c.Key("details")).FillWidth().Padding(8, 10).Gap(4).Radius(6).Background(side).Children(func() {
				ui.Text(c, question.displayTitle()).Selectable().FontSize(viewport.Font(c, 12)).FontWeight(620).TextColor(ink).FillWidth()
				if approval := question.Approval; approval != nil {
					if approval.Target != "" {
						ui.Text(c, approval.Target).Selectable().FontSize(viewport.Font(c, 12)).TextColor(muted).FillWidth()
					}
					if approval.Preview != "" {
						scroll, offset := elastic.Scroll(c)
						scroll.FillWidth().MaxHeight(140).Children(func() {
							offset.Apply(ui.Text(c, approval.Preview)).Selectable().Font("monospace").FontSize(viewport.Font(c, 12)).TextColor(muted).FillWidth()
						})
					}
				}
				ui.Text(c, question.displayAnswer()).Selectable().FontSize(viewport.Font(c, 12)).TextColor(ink).FillWidth()
			})
		}
	})
}

func replyStatus(entry *turn, now time.Time) string {
	var label string
	switch entry.State {
	case "waiting":
		label = locales.Waiting()
	case "stopped":
		label = locales.Stopped()
	case "steered":
		label = locales.Steered()
	case "error":
		label = locales.GenerationFailed()
	}

	end := entry.FinishedAt
	if entry.State == "pending" {
		end = now
	}
	if entry.StartedAt.IsZero() || end.IsZero() {
		return label
	}
	duration := formatReplyDuration(end.Sub(entry.StartedAt))
	if label == "" {
		return locales.Elapsed(duration)
	}
	return locales.StatusElapsed(label, duration)
}

func formatReplyDuration(duration time.Duration) string {
	seconds := max(1, int(duration/time.Second))
	switch {
	case seconds >= 3600:
		return locales.DurationHours(seconds/3600, seconds/60%60, seconds%60)
	case seconds >= 60:
		return locales.DurationMinutes(seconds/60, seconds%60)
	default:
		return locales.DurationSeconds(seconds)
	}
}
