package chat

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/theme"
)

func (app *chatApp) statisticsSettings(c *ui.Context) {
	label := locales.ShowStatistics()
	enabled := !app.desktop.HideStatistics
	settings.AboutRow(ui.Row(c)).Justify(ui.SpaceBetween).PaddingY(10).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, label).FontSize(15).FontWeight(660)
			ui.Text(c, locales.ShowStatisticsHint()).FontSize(12).TextColor(muted)
		})
		theme.Switch(c.Key("switch"), &enabled, label, func() {
			preferences := app.desktop
			preferences.HideStatistics = !enabled
			if app.saveDesktopPreferences(preferences) {
				app.form.generalStatus = settingsStatus{}
			}
		})
	})
}

func metricNumber(value int) string {
	return i18n.FormatNumber(i18n.Locale(), value, i18n.NumberOptions{})
}

func metricDecimal(value float64) string {
	return i18n.FormatNumber(i18n.Locale(), value, i18n.NumberOptions{MaxFractionDigits: 1})
}

func metricDuration(duration time.Duration) string {
	switch {
	case duration < time.Second:
		return locales.StatisticsMilliseconds(metricNumber(int(duration.Round(time.Millisecond) / time.Millisecond)))
	case duration < time.Minute:
		return locales.StatisticsSeconds(metricDecimal(duration.Seconds()))
	default:
		return formatReplyDuration(duration)
	}
}

type statisticLine struct{ label, value string }

type statisticCard struct {
	glyph, title, summary, total, note string
	lines                              []statisticLine
}

func (m usageMetrics) tokenCard(session bool) statisticCard {
	card := statisticCard{glyph: "database", title: locales.Statistics(), summary: locales.Statistics()}
	if m.UsageSamples > 0 {
		card.total = locales.StatisticsTokenTotal(metricNumber(m.Input + m.Output))
		card.summary = card.total
		card.lines = append(card.lines, statisticLine{locales.StatisticsInput(), metricNumber(m.Input)})
		if m.Input > 0 && m.Cached > 0 {
			cacheHit := i18n.FormatNumber(i18n.Locale(), float64(m.Cached)/float64(m.Input), i18n.NumberOptions{Percent: true, MaxFractionDigits: 1})
			card.lines = append(card.lines, statisticLine{locales.StatisticsCache(), metricNumber(m.Cached) + " · " + cacheHit})
			if session {
				card.summary += " · " + locales.StatisticsCacheHit(cacheHit)
			}
		}
		card.lines = append(card.lines, statisticLine{locales.StatisticsOutput(), metricNumber(m.Output)})
		if m.Reasoning > 0 {
			card.lines = append(card.lines, statisticLine{locales.StatisticsReasoning(), metricNumber(m.Reasoning)})
		}
	}
	if m.UsageSamples == 0 {
		card.note = locales.StatisticsUsageUnavailable()
	} else if m.UsageSamples < m.Requests {
		card.note = locales.StatisticsUsagePartial()
	}
	return card
}

func (m usageMetrics) timingCard(elapsed time.Duration) statisticCard {
	title := locales.StatisticsTiming()
	card := statisticCard{glyph: "clock", title: title, summary: title}
	if elapsed > 0 {
		duration := metricDuration(elapsed)
		card.summary = locales.Elapsed(duration)
		card.lines = append(card.lines, statisticLine{locales.StatisticsReplyTime(), duration})
	}
	card.lines = append(card.lines, statisticLine{locales.StatisticsModelTime(), metricDuration(m.ModelTime)})
	if m.DecodeTime > 0 {
		card.lines = append(card.lines, statisticLine{locales.StatisticsGenerationSpeed(), m.speed()})
	}
	if m.FirstTokens > 0 {
		card.lines = append(card.lines, statisticLine{locales.StatisticsFirstToken(), metricDuration(m.FirstTokenTime / time.Duration(m.FirstTokens))})
	}
	card.lines = append(card.lines, statisticLine{locales.StatisticsRequests(), metricNumber(m.Requests)})
	return card
}

func (m usageMetrics) speed() string {
	return locales.StatisticsSpeed(metricDecimal(float64(m.DecodedTokens) / m.DecodeTime.Seconds()))
}

func replyElapsed(entry *turn) time.Duration {
	if entry.StartedAt.IsZero() || entry.FinishedAt.IsZero() {
		return 0
	}
	return entry.FinishedAt.Sub(entry.StartedAt)
}

func (app *chatApp) replyFooter(c *ui.Context, chat *conversation, entry *turn, latest, showActions, showStatistics bool, elapsed time.Duration) {
	row := ui.Row(c).Gap(4).AlignItems(ui.Center).Wrap()
	open := ui.Local(row, "panels", func() statisticOpen { return statisticOpen{} })
	if !entry.FinishedAt.IsZero() {
		row.MinHeight(25)
	}
	row.Children(func() {
		actions := ui.Row(c.Key("actions")).Gap(4).AlignItems(ui.Center)
		if !showActions {
			actions.Opacity(0)
		}
		actions.Children(func() {
			if entry.Content != "" {
				copyMessageIcon(c, entry, locales.Copy(), locales.Copied())
			}
			if latest {
				messageIcon(c, locales.Retry(), "retry", chat.isRunning(), func() { app.retryReply = entry })
			}
		})
		if showStatistics && (showActions || open.timing || open.usage) {
			ui.Row(c.Key("statistics")).Gap(3).AlignItems(ui.Center).Children(func() {
				statisticButton(c.Key("usage"), entry.Metrics.tokenCard(false), &open.usage, &open.timing)
				statisticButton(c.Key("timing"), entry.Metrics.timingCard(elapsed), &open.timing, &open.usage)
			})
		}
		if showActions {
			messageTimestamp(c.Key("completion-time"), entry.FinishedAt)
		}
	})
}

func (app *chatApp) conversationStatistics(c *ui.Context, chat *conversation) {
	if app.desktop.HideStatistics || chat == nil {
		return
	}
	metrics := chat.metrics()
	if metrics.Requests == 0 {
		return
	}
	var replies int
	for _, entry := range chat.Turns {
		if entry.Metrics.Requests > 0 {
			replies++
		}
	}
	timing := metrics.timingCard(0)
	timing.title = locales.StatisticsSessionTiming()
	timing.glyph = "gauge"
	timing.summary = locales.StatisticsSessionCounts(replies, metrics.Requests)
	if metrics.DecodeTime > 0 {
		timing.summary += " · " + metrics.speed()
	}
	timing.lines = append([]statisticLine{{locales.StatisticsReplies(), metricNumber(replies)}}, timing.lines...)
	usage := metrics.tokenCard(true)
	usage.title = locales.StatisticsSessionUsage()
	row := ui.Row(c.Key(chat.ID)).AlignItems(ui.Center).Justify(ui.Center).Wrap().Gap(4)
	open := ui.Local(row, "panels", func() statisticOpen { return statisticOpen{} })
	row.Children(func() {
		statisticButton(c.Key("timing"), timing, &open.timing, &open.usage)
		statisticButton(c.Key("usage"), usage, &open.usage, &open.timing)
	})
}

type statisticOpen struct{ timing, usage bool }

func statisticButton(c *ui.Context, card statisticCard, open, other *bool) {
	button := pointerButton(c, false).Label(card.title).Expanded(*open).
		Height(24).PaddingX(7).AlignItems(ui.Center).Gap(5).Radius(12)
	color := muted
	if button.Hovered() || *open {
		button.Background(side)
		color = ink
	}
	button.Children(func() {
		icon(c, card.glyph, color, 13)
		ui.Text(c, card.summary).FontSize(11).TextColor(color).SingleLine().Ellipsis("…")
	}).OnClick(func() {
		*open = !*open
		*other = false
	})
	ui.PopoverBase(c.Key("detail"), button, open, func(panel ui.Element) {
		panel.AttachTo(button, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6).
			AttachEdgeMargin(12).Width(288).Padding(14).Gap(9).
			Background(white).Border(1, border).Radius(12).Shadow(0, 5, 18, 0, ui.RGBA(20, 34, 28, .14)).
			Label(card.title)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(7).Children(func() {
			icon(c, card.glyph, muted, 14)
			ui.Text(c, card.title).FontSize(12).FontWeight(630).Grow(1)
			if card.total != "" {
				ui.Text(c, card.total).FontSize(12).FontWeight(630).Selectable()
			}
		})
		ui.Box(c).FillWidth().Height(1).Background(border)
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			for _, line := range card.lines {
				ui.Row(c.Key(line.label)).FillWidth().AlignItems(ui.Start).Gap(12).Children(func() {
					ui.Text(c, line.label).Grow(1).MinWidth(0).FontSize(12).TextColor(muted)
					ui.Text(c, line.value).FontSize(12).Selectable()
				})
			}
			if card.note != "" {
				ui.Text(c, card.note).FillWidth().FontSize(11).TextColor(muted)
			}
		})
	})
}
