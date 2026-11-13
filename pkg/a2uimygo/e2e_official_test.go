package a2uimygo_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
	"github.com/ggdream/microlight/pkg/a2uimygo"
	"gopkg.in/yaml.v3"
)

func TestOfficialMyGoConversation(t *testing.T) {
	files, err := filepath.Glob("testdata/official/e2e/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("missing conversation fixtures: %v", err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Name    string
			Formats []a2uiagent.Format
			Turns   []struct {
				Chunks   map[a2uiagent.Format][]string `yaml:"modelChunks"`
				Wire     []map[string]any              `yaml:"expectWire"`
				Trees    map[string]*adapterNode       `yaml:"expectViewTree"`
				Outbound []map[string]any              `yaml:"expectOutbound"`
				Data     map[string]map[string]any     `yaml:"expectDataModel"`
				Ops      []struct {
					Op      string
					Surface string `yaml:"surfaceId"`
					Target  string `yaml:"targetNode"`
					Value   string `yaml:"inputValue"`
				} `yaml:"uiOps"`
			}
		}
		if err := yaml.Unmarshal(raw, &cases); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			for _, format := range tc.Formats {
				t.Run(tc.Name+"/"+string(format), func(t *testing.T) {
					catalog, err := a2ui.ParseCatalog(a2ui.BasicCatalogDocument())
					if err != nil {
						t.Fatal(err)
					}
					var p a2ui.Processor
					p.RegisterCatalog(catalog)
					var actions []a2ui.RendererMessage
					surfaceID := ""
					tester := ui.NewTester(func(c *ui.Context) {
						a2uimygo.View(c, p.Surface(surfaceID), a2uimygo.Options{
							OnAction: func(event a2ui.RendererMessage) { actions = append(actions, event) },
						})
					}, 820, 640)
					for index, turn := range tc.Turns {
						actions = make([]a2ui.RendererMessage, 0)
						stream := a2uiagent.Stream{State: &p, Format: format, Catalog: catalog}
						var prose strings.Builder
						consume := func(parts []a2uiagent.Part, err error) {
							if err != nil {
								var integrity *a2ui.IntegrityError
								if !errors.As(err, &integrity) {
									t.Fatalf("turn %d: %v", index+1, err)
								}
								actions = append(actions, integrity.RendererMessage())
							}
							for _, part := range parts {
								prose.WriteString(part.Text)
							}
						}
						for _, chunk := range turn.Chunks[format] {
							consume(stream.Push(chunk))
						}
						consume(stream.Close())
						var expectedText []string
						for _, part := range turn.Wire {
							if text, ok := part["text"].(string); ok {
								expectedText = append(expectedText, text)
							}
						}
						adapterEqual(t, strings.TrimSpace(prose.String()), strings.Join(expectedText, ""))
						for id, tree := range turn.Trees {
							surfaceID = id
							tester.Frame()
							if tree == nil {
								if p.Surface(id) != nil {
									t.Fatalf("unexpected surface %s", id)
								}
								continue
							}
							assertAdapterNode(t, tester, p.Surface(id), *tree, make(map[string]bool))
						}
						for _, op := range turn.Ops {
							surfaceID = op.Surface
							tester.Frame()
							surface := p.Surface(surfaceID)
							node, ok := surface.Component(op.Target)
							if !ok {
								t.Fatalf("unknown target %s", op.Target)
							}
							if err := tester.Click(adapterLabel(surface, node)); err != nil {
								t.Fatal(err)
							}
							if op.Op == "user_input" {
								tester.Command("selectAll")
								tester.Type(op.Value)
							}
							tester.Frame()
						}
						for id, want := range turn.Data {
							actual, _ := p.Surface(id).Data().Get("")
							adapterEqual(t, actual, want)
						}
						if turn.Outbound != nil {
							raw, _ := json.Marshal(actions)
							var actual []map[string]any
							_ = json.Unmarshal(raw, &actual)
							if len(actual) != len(turn.Outbound) {
								t.Fatalf("got %s, want %v", raw, turn.Outbound)
							}
							for i, event := range actual {
								if reported, ok := event["error"].(map[string]any); ok {
									want := turn.Outbound[i]["error"].(map[string]any)
									for key, value := range want {
										if match, ok := value.(map[string]any); ok && match["$match"] == "any" {
											if value, ok := reported[key].(string); !ok || value == "" {
												t.Fatalf("missing %s: %v", key, reported)
											}
											reported[key] = match
										}
									}
									continue
								}
								action := event["action"].(map[string]any)
								want := turn.Outbound[i]["action"].(map[string]any)
								if _, wildcard := want["timestamp"].(map[string]any); wildcard {
									if _, ok := action["timestamp"].(string); !ok {
										t.Fatalf("missing action timestamp: %v", action)
									}
									action["timestamp"] = want["timestamp"]
								}
							}
							adapterEqual(t, actual, turn.Outbound)
						}
					}
				})
			}
		}
	}
}
