package a2ui_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ggdream/microlight/pkg/a2ui"
)

func TestOfficialFormMessages(t *testing.T) {
	const stream = `{"version":"1.0","createSurface":{"surfaceId":"test","dataModel":{"items":[{"name":"Old"},{"name":"Other"}]}}}
{"version":"1.0","updateComponents":{"surfaceId":"test","components":[{"id":"root","component":"Button","child":"label","action":{"event":{"name":"save","context":{"name":{"@path":"name"}}}}},{"id":"label","component":"Text","text":{"@path":"/items/0/name"}}]}}
{"version":"1.0","updateDataModel":{"surfaceId":"test","path":"/items/0/name","value":"New"}}`
	var processor a2ui.Processor
	if err := a2ui.ReadJSONL(strings.NewReader(stream), processor.Apply); err != nil {
		t.Fatal(err)
	}
	surface := processor.Surface("test")
	label, _ := surface.Component("label")
	if got := surface.String(label.Props["text"], ""); got != "New" {
		t.Fatalf("resolved label = %q", got)
	}
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	action, err := surface.Action("root", "/items/0", now)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":"1.0","action":{"name":"save","surfaceId":"test","sourceComponentId":"root","timestamp":"2026-11-13T00:00:00Z","context":{"name":"New"}}}`
	if string(data) != want {
		t.Fatalf("action = %s", data)
	}
	if err := a2ui.ReadJSONL(strings.NewReader(`{"version":"1.0","updateDataModel":{"surfaceId":"test","path":"/items/0","value":null}}
{"version":"1.0","deleteSurface":{"surfaceId":"test"}}`), func(message a2ui.AgentMessage) error {
		if err := processor.Apply(message); err != nil {
			return err
		}
		if message.Data != nil {
			if other, _ := surface.Data().Get("/items/1/name"); other != "Other" {
				t.Fatal("deleting a list slot changed a sibling binding")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if processor.Surface("test") != nil {
		t.Fatal("surface remained after deletion")
	}
}
