package markup

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func TestMermaidDiagramTypes(t *testing.T) {
	for name, source := range map[string]string{
		"flowchart": "flowchart TD\n A[开始] --> B{校验}\n B -->|通过| C[完成]",
		"sequence":  "sequenceDiagram\n participant A as 客户端\n participant B as 服务端\n A->>B: 提交请求",
		"state":     "stateDiagram-v2\n [*] --> Ready\n Ready --> Done",
	} {
		t.Run(name, func(t *testing.T) {
			diagram, err := renderMermaid(source)
			if err != nil || diagram.shapes == nil || len(diagram.labels) == 0 {
				t.Fatalf("expected shapes and text layouts, got %d labels, error %v", len(diagram.labels), err)
			}
		})
	}
}

func TestMermaidMarkdownStreamingAndFallback(t *testing.T) {
	options := New(ui.RGB(0, 0, 0))
	source := "flowchart LR\n A[提交] --> B[完成]\n"
	message := "```mermaid\n" + source
	view := ui.NewTester(func(c *ui.Context) { markdown.Stream(c, message, options) }, 540, 380)
	if view.HasText("Mermaid") {
		t.Fatal("an open fence should remain source")
	}
	message += "```\n"
	deadline := time.Now().Add(3 * time.Second)
	for !view.HasText("Mermaid") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		view.Advance(100 * time.Millisecond)
	}
	if !view.HasText("Mermaid") {
		t.Fatalf("closed Mermaid block did not render: %v", view.Texts())
	}
	if err := view.Click("Copy code"); err != nil {
		t.Fatal(err)
	}
	if got := view.Clipboard(); got != source {
		t.Fatalf("copy returned %q, want %q", got, source)
	}

	message = "```mermaid\nunsupportedDiagram\n```\n"
	view.Frame()
	if !view.HasText("unsupportedDiagram\n") && !view.HasText("unsupportedDiagram") {
		t.Fatalf("invalid diagram did not fall back to source: %v", view.Texts())
	}
}
