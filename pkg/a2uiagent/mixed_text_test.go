package a2uiagent

import (
	"strings"
	"testing"
)

func TestMixedStreamPreservesCodeAndRejectsIncompleteBlocks(t *testing.T) {
	stream, err := (Config{ContinueAfterError: true}).NewStream(Mixed, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	const prose = "Use `<a2ui>` here.\n\n```text\n<a2ui-json>example</a2ui-json>\n```\n\n中文\n"
	const wire = "<a2ui>surface(\"live\")\nroot = Text(\"Ready\")</a2ui>\n<a2ui>surface(\"unfinished\")"
	var text strings.Builder
	var rejected int
	var complete int
	consume := func(parts []Part, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range parts {
			text.WriteString(part.Text)
			if part.Err != nil {
				rejected++
			}
			if len(part.Messages) > 0 {
				complete++
			}
		}
	}
	source := prose + wire
	for i := range len(source) {
		consume(stream.Push(source[i : i+1]))
	}
	consume(stream.Close())
	if text.String() != prose+"\n" || rejected != 1 || complete != 1 ||
		stream.State.Surface("live") == nil || stream.State.Surface("unfinished") != nil {
		t.Fatalf("wrong split: %q, %d complete, %d rejected", text.String(), complete, rejected)
	}
}
