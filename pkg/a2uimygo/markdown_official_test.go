package a2uimygo_test

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
	"gopkg.in/yaml.v3"
)

func TestOfficialMyGoMarkdown(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/markdown_subset.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string
		Markdown string
		Text     string `yaml:"expectVisibleText"`
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			node, err := a2ui.NewComponent(a2ui.Root, "Text", map[string]any{"text": tc.Markdown})
			if err != nil {
				t.Fatal(err)
			}
			var p a2ui.Processor
			if err := p.Apply(a2ui.AgentMessage{Version: a2ui.Version, Create: &a2ui.CreateSurface{
				SurfaceRef: a2ui.SurfaceRef{ID: "markdown"},
				Catalog:    a2ui.BasicCatalog,
				Components: []a2ui.Component{node},
			}}); err != nil {
				t.Fatal(err)
			}
			tester := ui.NewTester(func(c *ui.Context) {
				a2uimygo.View(c, p.Surface("markdown"), a2uimygo.Options{})
			}, 600, 240)
			tester.Frame()
			for _, value := range tester.Texts() {
				if strings.Join(strings.Fields(value), " ") == tc.Text {
					if len(tester.OpenedURLs()) != 0 {
						t.Fatal("rendering Markdown opened a URL")
					}
					return
				}
			}
			t.Fatalf("visible text %v, want %q", tester.Texts(), tc.Text)
		})
	}
}
