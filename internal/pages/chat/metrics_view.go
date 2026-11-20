package chat

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/viewport"
)

func (app *chatApp) statisticsSettings(c *ui.Context) {
	label := locales.ShowStatistics()
	enabled := !app.desktop.HideStatistics
	settings.AboutRow(c, ui.Row(c)).Justify(ui.SpaceBetween).PaddingY(10).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, label).FontSize(viewport.Font(c, 15)).FontWeight(660)
			ui.Text(c, locales.ShowStatisticsHint()).FontSize(viewport.Font(c, 12)).TextColor(muted)
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

func (m usageMetrics) tokenCard() statisticCard {
	card := statisticCard{glyph: "database", title: locales.Statistics(), summary: locales.Statistics()}
	if m.UsageSamples > 0 {
		card.total = locales.StatisticsTokenTotal(metricNumber(m.Input + m.Output))
		card.summary = card.total
		card.lines = append(card.lines, statisticLine{locales.StatisticsInput(), metricNumber(m.Input)})
		if m.Input > 0 && m.Cached > 0 {
			cacheHit := i18n.FormatNumber(i18n.Locale(), float64(m.Cached)/float64(m.Input), i18n.NumberOptions{Percent: true, MaxFractionDigits: 1})
			card.lines = append(card.lines, statisticLine{locales.StatisticsCache(), metricNumber(m.Cached) + " · " + cacheHit})
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

func (app *chatApp) replyFooter(c *ui.Context, chat *conversation, entry *turn, allowRetry, showActions, showStatistics bool, elapsed time.Duration) {
	layout := windowLayout(c)
	gap := layout.Density(4, layout.X(1))
	row := ui.Row(c).Gap(gap).AlignItems(ui.Center).Wrap()
	if layout.Compact() {
		row.Margin(-layout.Y(6), 0, 0)
	}
	preview := ui.Local(row, "preview", func() statisticPreview { return statisticPreview{} })
	if !entry.FinishedAt.IsZero() {
		row.MinHeight(25)
	}
	row.Children(func() {
		actions := ui.Row(c.Key("actions")).Gap(gap).AlignItems(ui.Center)
		if !showActions {
			actions.Opacity(0)
		}
		actions.Children(func() {
			if entry.Content != "" {
				copyMessageIcon(c, entry, locales.Copy(), locales.Copied())
			}
			if allowRetry {
				messageIcon(c, locales.Retry(), "retry", chat.isRunning() || chat.waitingReply() != nil, func() { app.retryReply = entry })
			}
		})
		if showStatistics && showActions {
			stats := c.Key("statistics")
			ui.Row(stats).Gap(layout.Density(3, layout.X(1))).AlignItems(ui.Center).Children(func() {
				preview.triggers(stats, [2]statisticCard{entry.Metrics.tokenCard(), entry.Metrics.timingCard(elapsed)}, false)
			})
		} else {
			*preview = statisticPreview{}
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
	timing.summary = timing.title
	if metrics.DecodeTime > 0 {
		timing.summary = metrics.speed()
	} else if metrics.ModelTime > 0 {
		timing.summary = locales.Elapsed(metricDuration(metrics.ModelTime))
	}
	timing.lines = append([]statisticLine{{locales.StatisticsReplies(), metricNumber(replies)}}, timing.lines...)
	usage := metrics.tokenCard()
	usage.title = locales.StatisticsSessionUsage()
	row := ui.Row(c.Key(chat.ID)).FillWidth().MaxWidth(conversationWidth).
		AlignItems(ui.Center).Justify(ui.Center).Gap(4)
	if app.composerLayout(c).Compact() {
		row.Gap(2)
	}
	preview := ui.Local(row, "preview", func() statisticPreview { return statisticPreview{} })
	row.Children(func() {
		preview.triggers(c, [2]statisticCard{timing, usage}, true)
	})
}

type statisticPreview struct {
	dismissed, selected int
}

func (p *statisticPreview) triggers(c *ui.Context, cards [2]statisticCard, session bool) {
	var anchors [2]ui.Element
	target := p.selected
	tap := touchControls(c)
	for i, card := range cards {
		anchor := statisticTrigger(c.Key(card.glyph), card, session)
		anchors[i] = anchor
		if tap && anchor.Clicked() {
			if target == i+1 {
				target = 0
			} else {
				target = i + 1
			}
			p.selected = target
		} else if !tap && (anchor.Hovered() || (anchor.FocusVisible() && target == 0)) {
			target = i + 1
		}
	}
	if target != p.dismissed {
		p.dismissed = 0
	}
	if target == 0 || target == p.dismissed || !c.WindowActive() {
		return
	}

	anchor, card := anchors[target-1], cards[target-1]
	open := true
	ui.PopoverBase(c.Key("detail"), anchor, &open, func(panel ui.Element) {
		// Only the trigger owns the preview; the card cannot keep itself hovered or focused.
		if !tap {
			panel.IgnoreInput(true)
		}
		statisticDetail(c.Key(card.glyph), panel, anchor, card)
	})
	if !open {
		p.dismissed = target
		p.selected = 0
	}
}

func statisticTrigger(c *ui.Context, card statisticCard, showSummary bool) ui.Element {
	trigger := ui.Row(c).Focusable().Label(card.title)
	if touchControls(c) {
		trigger = pointerButton(c, false).Label(card.title)
	}
	iconSize := float32(14)
	if showSummary {
		trigger.Height(24).MinWidth(0).MaxWidthPercent(50).Shrink(1).PaddingX(6).
			AlignItems(ui.Center).Gap(4).Radius(messageControlRadius)
		if windowLayout(c).Compact() {
			trigger.Height(26).PaddingX(4).Gap(3)
		}
		iconSize = 13
	} else {
		messageIconFrame(trigger)
		if touchControls(c) {
			trigger.Size(messageTouchWidth(c), 44).Shrink(0).Center()
		}
	}
	color := muted
	active := trigger.Hovered() || trigger.FocusVisible() || trigger.Pressed()
	if active {
		if showSummary || !touchControls(c) {
			trigger.Background(hover)
		}
		color = ink
	}
	return trigger.Children(func() {
		if !showSummary && touchControls(c) {
			face := ui.Box(c).Size(messageTouchWidth(c), messageTouchWidth(c)).Radius(messageControlRadius).Center()
			if active {
				face.Background(hover)
			}
			face.Children(func() { icon(c, card.glyph, color, iconSize) })
		} else {
			icon(c, card.glyph, color, iconSize)
		}
		if showSummary {
			ui.Text(c, card.summary).MinWidth(0).FontSize(viewport.Font(c, 11)).TextColor(color).SingleLine().Ellipsis("…")
		}
	})
}

func statisticDetail(c *ui.Context, panel, anchor ui.Element, card statisticCard) {
	layout := windowLayout(c)
	padding := layout.Y(layout.Density(14, 10))
	height := popoverHeight(c, anchor, layout.FitHeight(480, 12))
	if touchControls(c) {
		panel.MaxHeight(height)
	}
	material3.Elevate(material3.Menu(panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6).
		AttachEdgeMargin(12).Width(layout.FitWidth(288, 12)).Padding(padding).Gap(layout.Y(9)),
	).Radius(material3.ShapeMedium), 2).
		Label(card.title)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(7).Children(func() {
		icon(c, card.glyph, muted, 14)
		ui.Text(c, card.title).FontSize(viewport.Font(c, 12)).FontWeight(630).Grow(1)
		if card.total != "" {
			ui.Text(c, card.total).FontSize(viewport.Font(c, 12)).FontWeight(630)
		}
	})
	material3.Divider(c, material3.DividerOptions{Space: 1})
	details := func() {
		for _, line := range card.lines {
			ui.Row(c.Key(line.label)).FillWidth().AlignItems(ui.Start).Gap(12).Children(func() {
				ui.Text(c, line.label).Grow(1).MinWidth(0).FontSize(viewport.Font(c, 12)).TextColor(muted)
				ui.Text(c, line.value).FontSize(viewport.Font(c, 12))
			})
		}
		if card.note != "" {
			ui.Text(c, card.note).FillWidth().FontSize(viewport.Font(c, 11)).TextColor(muted)
		}
	}
	if touchControls(c) {
		scroll, offset := elastic.Scroll(c.Key("detail-scroll"))
		scroll.FillWidth().MinHeight(0).Shrink(1).MaxHeight(max(44, height-2*padding-layout.Y(42))).Children(func() {
			offset.Apply(ui.Column(c)).FillWidth().Gap(layout.Y(6)).Children(details)
		})
	} else {
		ui.Column(c).FillWidth().Gap(layout.Y(6)).Children(details)
	}
}
