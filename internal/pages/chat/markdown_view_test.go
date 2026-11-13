package chat

import (
	"slices"
	"strings"
	"testing"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/pkg/markup"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func TestChatMarkdownRendersMathInDraftAndMessages(t *testing.T) {
	const source = "**公式示例** $x_1^2$ 和 \\(y^2\\)\n\n$$\n\\frac{a}{b}\n$$\n\n\\[\n\\begin{pmatrix}a&b\\\\c&d\\end{pmatrix}\n\\]"
	app := &chatApp{backend: &Chat{}, md: markup.New(ink)}
	inline := app.md.InlineMath
	renderedInline := map[string]bool{}
	app.md.InlineMath = func(tex string, size float32) (markdown.InlineObject, bool) {
		formula, ok := inline(tex, size)
		renderedInline[tex] = ok && formula.Width > 0 && formula.Height > 0 && formula.Draw != nil
		return formula, ok
	}
	assertMath := func(t *testing.T, surface *ui.Tester) {
		t.Helper()
		if !slices.ContainsFunc(surface.Texts(), func(text string) bool {
			return strings.Contains(text, "公式示例") && !strings.Contains(text, "**")
		}) {
			t.Fatal("Markdown emphasis was not parsed")
		}
		for _, text := range []string{`\frac{a}{b}`, `\begin{pmatrix}a&b\\c&d\end{pmatrix}`} {
			if _, found := surface.Find(text); !found {
				t.Fatalf("rendered element %q is missing: %v", text, surface.Texts())
			}
		}
		if !renderedInline["x_1^2"] || !renderedInline["y^2"] {
			t.Fatal("inline TeX was not drawn as a formula")
		}
		clear(renderedInline)
	}

	draft := ui.NewTester(func(c *ui.Context) { app.composer(c, nil) }, 680, 320)
	if err := draft.Click(locales.Message()); err != nil {
		t.Fatal(err)
	}
	draft.Type(source)
	draft.Frame()
	if raw := app.draft.Document.Text().String(); strings.Contains(raw, "**") || strings.Count(raw, "￼") != 4 {
		t.Fatalf("draft should display rich text and four math objects: %q", raw)
	}
	exported := app.draft.Markdown()
	for _, part := range []string{"**公式示例**", "$x_1^2$", `\(y^2\)`, `\frac{a}{b}`, `\begin{pmatrix}a&b\\c&d\end{pmatrix}`} {
		if !strings.Contains(exported, part) {
			t.Fatalf("draft serialization lost %q: %q", part, exported)
		}
	}
	clear(renderedInline)

	for _, role := range []string{"user", "assistant"} {
		t.Run(role, func(t *testing.T) {
			entry := &turn{message: message{Role: role, Content: source}, State: "done"}
			chat := &conversation{ID: role, Turns: []*turn{entry}}
			surface := ui.NewTester(func(c *ui.Context) { app.transcript(c, chat) }, 700, 410)
			assertMath(t, surface)
		})
	}
}

func TestComposerEditsRenderedFormulaInPlace(t *testing.T) {
	var input composer.Input
	md := markup.New(ink)
	submitted := ""
	surface := ui.NewTester(func(c *ui.Context) {
		editor := input.View(c, "消息", "", 140, md)
		if editor.Shortcut(0, ui.KeyEnter) {
			submitted = input.Markdown()
		}
	}, 520, 220)
	if err := surface.Click("消息"); err != nil {
		t.Fatal(err)
	}
	surface.Type("中文 $x^2$")
	if got := input.Document.Text().String(); got != "中文 ￼" {
		t.Fatalf("formula did not render in the editing surface: %q", got)
	}
	surface.Key(ui.Alt, ui.KeyEnter)
	if got := input.Document.Text().String(); got != "中文 $x^2$" {
		t.Fatalf("formula source did not open in place: %q", got)
	}
	surface.Type("x^3")
	surface.Key(0, ui.KeyRight)
	if got := input.Document.Text().String(); got != "中文 ￼" {
		t.Fatalf("edited formula did not render when the caret left it: %q", got)
	}
	surface.Key(0, ui.KeyEnter)
	if submitted != "中文 $x^3$" {
		t.Fatalf("Enter did not submit the edited source: %q", submitted)
	}
}
