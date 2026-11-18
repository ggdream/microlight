package chat

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
)

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
	if _, visible := view.Find(locales.Statistics()); visible {
		t.Fatal("the latest reply should also hide statistics at rest")
	}
	response, found := view.Find("Sample reply")
	if !found {
		t.Fatal("test reply is missing")
	}
	view.Move(response.X+4, response.Y+4)
	if err := view.Click(locales.Statistics()); err != nil {
		t.Fatal(err)
	}
	if !view.HasText(locales.StatisticsCache()) || !view.HasText(locales.StatisticsUsagePartial()) ||
		view.HasText(locales.StatisticsGenerationSpeed()) {
		t.Fatalf("unexpected reply usage details: %v", view.Texts())
	}

	if err := view.Click(locales.StatisticsTiming()); err != nil {
		t.Fatal(err)
	}
	if !view.HasText(locales.StatisticsGenerationSpeed()) || view.HasText(locales.StatisticsCache()) {
		t.Fatalf("switching to reply timing should close the usage popover: %v", view.Texts())
	}
	view.Key(0, ui.KeyEscape)
	if view.HasText(locales.StatisticsGenerationSpeed()) {
		t.Fatal("Escape did not close the timing popover")
	}

	if err := view.Click(locales.StatisticsSessionTiming()); err != nil {
		t.Fatal(err)
	}
	if !view.HasText(locales.StatisticsReplies()) || view.HasText(locales.StatisticsReplyTime()) {
		t.Fatalf("conversation timing should show aggregate metrics: %v", view.Texts())
	}

	app.desktop.HideStatistics = true
	hidden := ui.NewTester(func(c *ui.Context) { app.conversationStatistics(c, chat) }, 660, 420)
	if err := hidden.Click(locales.StatisticsSessionTiming()); err == nil {
		t.Fatal("conversation statistics are available when the preference is off")
	}
}
