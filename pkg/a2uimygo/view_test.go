package a2uimygo_test

import (
	"testing"
	"time"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestFormUsesCurrentValueForClickAndEnter(t *testing.T) {
	const form = `{"version":"1.0","createSurface":{"surfaceId":"example","components":[
		{"id":"root","component":"Row","children":["answer","submit"]},
		{"id":"answer","component":"TextField","label":"Answer","value":{"@path":"/answer"}},
		{"id":"submit","component":"Button","child":"label","action":{"event":{"name":"submit","context":{"answer":{"@path":"/answer"}}}}},
		{"id":"label","component":"Text","text":"Submit"}
	],"dataModel":{"answer":""}}}`
	message, err := a2ui.Decode([]byte(form))
	if err != nil {
		t.Fatal(err)
	}
	var processor a2ui.Processor
	if err := processor.Apply(message); err != nil {
		t.Fatal(err)
	}
	var events []string
	surface := processor.Surface("example")
	window := ui.NewTester(func(c *ui.Context) {
		a2uimygo.View(c, surface, a2uimygo.Options{
			EnterActions: map[string]string{"answer": "submit"},
			OnAction: func(message a2ui.RendererMessage) {
				events = append(events, message.Action.Context["answer"].(string))
			},
		})
	}, 480, 180)
	if err := window.Click("Answer"); err != nil {
		t.Fatal(err)
	}
	window.Type("First")
	if err := window.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	if err := window.Click("Answer"); err != nil {
		t.Fatal(err)
	}
	window.Key(0, ui.KeyEnd)
	window.Type(" answer")
	window.Key(0, ui.KeyEnter)
	if len(events) != 2 || events[0] != "First" || events[1] != "First answer" {
		t.Fatalf("unexpected submitted answers %v", events)
	}
}

func TestDateTimeFormSubmitsDefaultAndEditedDate(t *testing.T) {
	const form = `{"version":"1.0","createSurface":{"surfaceId":"example","components":[
		{"id":"root","component":"Column","children":["departure","submit"]},
		{"id":"departure","component":"DateTimeInput","label":"Departure","enableDate":true,"enableTime":true,"value":{"@path":"/departure"}},
		{"id":"submit","component":"Button","child":"label","action":{"event":{"name":"submit","context":{"departure":{"@path":"/departure"}}}}},
		{"id":"label","component":"Text","text":"Submit"}
	],"dataModel":{"departure":""}}}`
	msg, err := a2ui.Decode([]byte(form))
	if err != nil {
		t.Fatal(err)
	}
	var processor a2ui.Processor
	if err := processor.Apply(msg); err != nil {
		t.Fatal(err)
	}
	surface := processor.Surface("example")
	var submitted any
	window := ui.NewTester(func(c *ui.Context) {
		a2uimygo.View(c, surface, a2uimygo.Options{
			OnAction: func(message a2ui.RendererMessage) {
				submitted = message.Action.Context["departure"]
			},
		})
	}, 480, 440)
	window.SetRegion(i18n.Region{Locale: "en-US"})
	want := window.Now().Local().Truncate(time.Minute)
	const layout = "2006-01-02T15:04:05"
	if err := window.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	if submitted != want.Format(layout) {
		t.Fatalf("untouched date submitted %v, want %s", submitted, want.Format(layout))
	}
	format := func(value time.Time, skeleton string) string {
		return i18n.FormatDate("en-US", value, i18n.DateOptions{Skeleton: skeleton})
	}
	if err := window.Click(format(want, "yMd")); err != nil {
		t.Fatal(err)
	}
	next := want.AddDate(0, 0, 1)
	if err := window.Click(format(next, "yMMMMd")); err != nil {
		t.Fatal(err)
	}
	window.Advance(time.Minute)
	if err := window.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	if edited := next.Format(layout); submitted != edited {
		t.Fatalf("calendar edit submitted %v, want %s", submitted, edited)
	}
}
