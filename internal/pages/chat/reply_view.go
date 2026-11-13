package chat

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/chat/preview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func replyReasoning(entry *turn) (hasReasoning, open bool) {
	hasBody := strings.TrimSpace(entry.Content) != ""
	compressing := entry.State == "pending" && entry.compressing
	hasReasoning = compressing || len(entry.Activity) != 0 || strings.TrimSpace(entry.processPreview) != "" || slices.ContainsFunc(entry.Clarifications, func(q clarification) bool {
		return q.Answer != ""
	})
	open = (entry.State == "pending" || entry.State == "waiting") && !hasBody
	if entry.reasoningOpen != nil {
		open = *entry.reasoningOpen
	}
	return hasReasoning, open
}

func replyRunningStatus(entry *turn) string {
	if slices.ContainsFunc(entry.Activity, func(activity timeline.Entry) bool { return activity.Pending }) {
		return ""
	}
	if entry.compressing || strings.TrimSpace(entry.processPreview) != "" || strings.TrimSpace(entry.Content) != "" {
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
				ui.Text(c, label).FontSize(12).TextColor(muted)
				chevron := ui.Icon(c, icons.SVG("ios_chevron_right")).Size(16, 16).TextColor(muted)
				if open {
					chevron.Rotate(90)
				}
			}).OnClick(func() {
				show := !open
				entry.reasoningOpen = &show
			})
		case label != "":
			ui.Text(c, label).FontSize(12).TextColor(muted).Padding(3, 0)
		}
	})
}

func (app *chatApp) replyBody(c *ui.Context, entry *turn, showReasoning bool) {
	hasBody := strings.TrimSpace(entry.Content) != ""
	if !showReasoning && !hasBody {
		return
	}
	ui.Column(c.Key("reply-body")).FillWidth().Gap(5).Children(func() {
		if showReasoning {
			app.reasoningSegments(c, entry)
			if entry.State == "pending" && entry.compressing {
				ui.Row(c.Key("context-compression")).AlignItems(ui.Center).Gap(8).Padding(4, 0).Children(func() {
					icon(c, "document_text", muted, 16)
					ui.Text(c, locales.Compressing()).FontSize(12).TextColor(muted)
				})
			}
		}
		if hasBody {
			app.replyText(c.Key("answer"), entry.Content, entry.State == "pending")
		}
	})
}

func (app *chatApp) replyText(c *ui.Context, text string, streaming bool) {
	if streaming {
		markdown.Stream(c, text, app.md)
	} else {
		markdown.View(c, text, app.md)
	}
}

func (app *chatApp) reasoningSegments(c *ui.Context, entry *turn) {
	type answerKey int
	start := 0
	showActivity := func(end int) {
		for i, activity := range entry.Activity[start:end] {
			if activity.Tool == "" {
				app.replyText(c.Key(start+i), activity.Text, false)
				continue
			}
			app.toolActivityRow(c.Key(start+i), entry, start+i)
		}
		start = end
	}
	for i := range entry.Clarifications {
		question := &entry.Clarifications[i]
		if question.Answer == "" {
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
	if text := strings.TrimSpace(entry.processPreview); text != "" {
		app.replyText(c.Key("stream"), text, true)
	}
}

func (app *chatApp) toolActivityRow(c *ui.Context, entry *turn, index int) {
	activity := entry.Activity[index]
	target := activity.ShortTarget()
	command := activity.RunsCommand()
	fileTarget := target != "" && (activity.Tool == localtools.Read || activity.Tool == localtools.List || activity.Tool == localtools.Write)
	row := ui.Column(c).FillWidth().Gap(4)
	open := ui.Local(row, "tool-details-open", func() bool { return false })
	row.Children(func() {
		var line ui.Element
		if command {
			line = pointerButton(c.Key("toggle"), false).Label(locales.ToolArguments()).Expanded(*open).
				OnClick(func() { *open = !*open })
		} else {
			line = ui.Row(c)
		}
		line.FillWidth().Justify(ui.Start).AlignItems(ui.Center).Gap(8).Padding(4, 0)
		bodyStyle := markdown.DefaultTheme(c.Theme())
		hovered := command && line.Hovered()
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
					ui.Themed(c, conversationMenuTheme).Background(ui.Transparent).MinWidth(0).
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
						button.PopoverMenu(func(menu *ui.Menu) {
							if menu.Item(locales.OpenPreview()).Icon(icons.Menu("panel_left_expand")).Chosen() {
								app.openToolPreview(entry, index)
							}
							if full := activity.DisplayTarget(); full != "" {
								if menu.Item(locales.CopyTarget()).Icon(icons.Menu("copy")).Chosen() {
									c.WriteClipboard(full)
								}
							}
						})
					})
				}
			})
			if diff := activity.Diff; diff != nil {
				preview.DiffCounts(c.Key("diff"), diff)
			}
			if command {
				replyActivityChevron(c, hovered || line.FocusVisible(), *open, bodyStyle.Text)
			}
		})
		if command && *open {
			ui.Column(c.Key("details")).FillWidth().Padding(8, 10).Gap(4).Radius(6).Background(side).Children(func() {
				ui.Text(c, activity.Tool).Font("monospace").FontSize(12).Selectable().FillWidth()
				details := activity.CommandText()
				if details == "" {
					details = activity.Details()
				}
				if details != "" {
					ui.Text(c, details).Font("monospace").FontSize(12).Selectable().FillWidth()
				}
			})
		}
	})
}

func replyActivityChevron(c *ui.Context, visible, open bool, color ui.Color) {
	ui.Box(c.Key("spacer")).Grow(1)
	ui.Box(c.Key("arrow")).Size(23, 23).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		if visible || open {
			chevron := ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).TextColor(color)
			if open {
				chevron.Rotate(90)
			}
		}
	})
}

func answeredClarification(c *ui.Context, question *clarification) {
	title := locales.ClarificationRecord()
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
			replyActivityChevron(c, button.Hovered() || button.FocusVisible(), question.answerOpen, bodyStyle.Text)
		}).OnClick(func() {
			question.answerOpen = !question.answerOpen
		})
		if question.answerOpen {
			ui.Column(c.Key("details")).FillWidth().Padding(8, 10).Gap(4).Radius(6).Background(side).Children(func() {
				ui.Text(c, question.displayTitle()).Selectable().FontSize(12).FontWeight(620).TextColor(ink).FillWidth()
				if approval := question.Approval; approval != nil {
					if approval.Target != "" {
						ui.Text(c, approval.Target).Selectable().FontSize(12).TextColor(muted).FillWidth()
					}
					if approval.Preview != "" {
						ui.Scroll(c).FillWidth().MaxHeight(140).Children(func() {
							ui.Text(c, approval.Preview).Selectable().Font("monospace").FontSize(12).TextColor(muted).FillWidth()
						})
					}
				}
				ui.Text(c, question.displayAnswer()).Selectable().FontSize(12).TextColor(ink).FillWidth()
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
