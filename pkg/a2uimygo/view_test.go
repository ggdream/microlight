package a2uimygo_test

import (
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"

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
