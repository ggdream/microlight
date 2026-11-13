package chat

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/localtools"
)

func TestExpandedShellCardShowsAndCopiesTranscript(t *testing.T) {
	code := 0
	output := "from stdout\n  from stderr\n"
	entry := &turn{Activity: []timeline.Entry{{
		Tool: localtools.Execute, Arguments: `{"command":"cat","stdin":"given input"}`,
		Output: &output, CommandResult: &localtools.CommandResult{ExitCode: &code},
	}}}
	app := &chatApp{}
	surface := ui.NewTester(func(c *ui.Context) {
		app.toolActivityRow(c, entry, 0)
	}, 450, 320)
	if surface.HasText("Shell") {
		t.Fatal("card should start collapsed")
	}
	if err := surface.Click(locales.ToolArguments()); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Shell", "$ cat", "given input", "from stdout\n  from stderr", locales.ShellSuccess()} {
		if !surface.HasText(label) {
			t.Fatalf("missing %q in %v", label, surface.Texts())
		}
	}
	if err := surface.Click(locales.Copy()); err != nil {
		t.Fatal(err)
	}
	if got := surface.Clipboard(); got != "cat\ngiven input\n"+output {
		t.Fatalf("unexpected clipboard: %q", got)
	}
	if !surface.HasText(locales.Copied()) {
		t.Fatal("copy feedback is missing")
	}
}
