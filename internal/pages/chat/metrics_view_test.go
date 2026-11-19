package chat

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
)

func TestCompactStatisticDetailsScrollAboveKeyboard(t *testing.T) {
	card := statisticCard{title: "Usage detail"}
	for i := range 18 {
		card.lines = append(card.lines, statisticLine{fmt.Sprintf("Metric %d", i), "100"})
	}
	window := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Justify(ui.End).Children(func() {
			anchor := ui.Button(c, "Usage")
			open := true
			ui.PopoverBase(c, anchor, &open, func(panel ui.Element) {
				statisticDetail(c, panel, anchor, card)
			})
		})
	}, 320, 570)
	window.SetInsets(44, 8, 250, 8)
	window.Frame()
	first, ok := window.Find("Metric 0")
	if !ok || first.Y < 44 {
		t.Fatalf("usage details overlap the safe area: %+v", first)
	}
	window.Move(first.X+5, first.Y+5)
	window.Scroll(first.X+5, first.Y+5, 0, 600)
	window.Advance(time.Second)
	last, ok := window.Find("Metric 17")
	if !ok || last.Y < 44 || last.Y+last.H > 320 {
		t.Fatalf("the last metric must be reachable above the keyboard: %+v", last)
	}
}

func TestStatisticsPopoversSeparateUsageAndTiming(t *testing.T) {
	app := &chatApp{}
	reply := &turn{Metrics: usageMetrics{
		Requests: 2, UsageSamples: 1, Input: 100, Output: 20, Cached: 50,
		ModelTime: 2 * time.Second, FirstTokenTime: 500 * time.Millisecond, FirstTokens: 1,
		DecodeTime: time.Second, DecodedTokens: 20,
	}}
	chat := &conversation{ID: "statistics-test", Turns: []*turn{reply}}
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).FillWidth().FillHeight().Padding(20).Justify(ui.End).Children(func() {
			response := ui.Column(c).FillWidth()
			showActions := response.Hovered() || response.FocusWithin()
			response.Children(func() {
				ui.Text(c, "Sample reply")
				app.replyFooter(c.Key("reply"), chat, reply, true, showActions, !app.desktop.HideStatistics, 3*time.Second)
			})
			app.conversationStatistics(c.Key("conversation"), chat)
		})
	}, 660, 420)

	if view.HasText(locales.StatisticsCache()) || view.HasText(locales.StatisticsGenerationSpeed()) {
		t.Fatal("detail rows should start closed")
	}
	if !view.HasText(reply.Metrics.speed()) || !view.HasText(locales.StatisticsTokenTotal(metricNumber(120))) {
		t.Fatalf("conversation summary should show only speed and total tokens: %v", view.Texts())
	}
	if _, visible := view.Find(locales.Statistics()); visible {
		t.Fatal("the latest reply should also hide statistics at rest")
	}
	moveTo := func(label string) {
		t.Helper()
		rect, ok := view.Find(label)
		if !ok {
			t.Fatalf("cannot find %q", label)
		}
		view.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	}
	moveTo("Sample reply")
	totals := slices.DeleteFunc(view.Texts(), func(text string) bool {
		return text != locales.StatisticsTokenTotal(metricNumber(120))
	})
	if len(totals) != 1 ||
		view.HasText(locales.Elapsed(metricDuration(3*time.Second))) ||
		view.HasText(locales.StatisticsReplyTime()) || view.HasText(locales.StatisticsUsagePartial()) {
		t.Fatalf("hovering a reply should show statistic icons without numbers: %v", view.Texts())
	}
	moveTo(locales.Statistics())
	if !view.HasText(locales.StatisticsCache()) || !view.HasText(locales.StatisticsUsagePartial()) ||
		view.HasText(locales.StatisticsGenerationSpeed()) {
		t.Fatalf("unexpected reply usage details: %v", view.Texts())
	}
	moveTo(locales.StatisticsCache())
	if view.HasText(locales.StatisticsCache()) {
		t.Fatal("reply details should close on leaving the icon, even over the card")
	}
	moveTo("Sample reply")

	moveTo(locales.StatisticsTiming())
	if !view.HasText(locales.StatisticsGenerationSpeed()) || view.HasText(locales.StatisticsCache()) {
		t.Fatalf("switching to reply timing should close the usage popover: %v", view.Texts())
	}
	moveTo("Sample reply")
	if view.HasText(locales.StatisticsGenerationSpeed()) {
		t.Fatal("moving back to the reply should close timing details")
	}
	moveTo(locales.StatisticsTiming())
	view.Key(0, ui.KeyEscape)
	if view.HasText(locales.StatisticsGenerationSpeed()) {
		t.Fatal("Escape did not close the timing popover")
	}

	moveTo(locales.StatisticsSessionTiming())
	if !view.HasText(locales.StatisticsReplies()) || view.HasText(locales.StatisticsReplyTime()) {
		t.Fatalf("conversation timing should show aggregate metrics: %v", view.Texts())
	}
	moveTo(locales.StatisticsSessionUsage())
	if !view.HasText(locales.StatisticsCache()) || view.HasText(locales.StatisticsReplies()) {
		t.Fatalf("moving to conversation usage should replace timing details: %v", view.Texts())
	}
	moveTo(locales.StatisticsCache())
	if view.HasText(locales.StatisticsCache()) {
		t.Fatal("conversation usage should close on leaving the trigger, even over the card")
	}
	moveTo(locales.StatisticsSessionTiming())
	view.Move(2, 2)
	if view.HasText(locales.StatisticsReplies()) {
		t.Fatal("conversation timing should close without clicking after the pointer leaves")
	}

	compact := ui.NewTester(func(c *ui.Context) { app.conversationStatistics(c, chat) }, 320, 420)
	timing, timingVisible := compact.Find(locales.StatisticsSessionTiming())
	usage, usageVisible := compact.Find(locales.StatisticsSessionUsage())
	if !timingVisible || !usageVisible || timing.Y != usage.Y || timing.H > 28 || usage.H > 28 {
		t.Fatalf("compact summary should fit on one short line: timing=%+v usage=%+v", timing, usage)
	}

	app.desktop.HideStatistics = true
	hidden := ui.NewTester(func(c *ui.Context) { app.conversationStatistics(c, chat) }, 660, 420)
	if err := hidden.Click(locales.StatisticsSessionTiming()); err == nil {
		t.Fatal("conversation statistics are available when the preference is off")
	}
}
