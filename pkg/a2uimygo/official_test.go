package a2uimygo_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"

	"github.com/egoist/mygo/ui"
)

func TestOfficialGallery(t *testing.T) {
	files, err := filepath.Glob("testdata/official/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var gallery struct{ CustomComponents []json.RawMessage }
			if err := json.Unmarshal(data, &gallery); err != nil {
				t.Fatal(err)
			}
			if len(gallery.CustomComponents) != 0 {
				t.Skip("fixture requires its own custom catalog implementation")
			}
			messages, err := a2ui.DecodeBatch(data)
			if err != nil {
				t.Fatal(err)
			}
			var p a2ui.Processor
			if err := p.ApplyBatch(messages); err != nil {
				t.Fatal(err)
			}
			for _, message := range messages {
				if message.Create == nil {
					continue
				}
				surface := p.Surface(message.Create.ID)
				if surface == nil {
					continue
				}
				ui.NewTester(func(c *ui.Context) { a2uimygo.View(c, surface, a2uimygo.Options{}) }, 800, 600)
			}
		})
	}
}

func TestOfficialScopedRendering(t *testing.T) {
	for _, suite := range []string{"index_function", "multi_catalog"} {
		data, err := os.ReadFile("testdata/official/" + suite + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Name     string
			Action   string
			Catalogs []map[string]any
			Steps    []struct {
				Payload     []map[string]any
				ExpectError map[string]any `yaml:"expectError"`
			}
			Expect struct {
				Surfaces map[string]struct {
					Components map[string]struct{ Text string }
				}
			}
		}
		if err := yaml.Unmarshal(data, &cases); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			// The select_catalog fixtures exercise an adapter API and use
			// synthetic surface capabilities rather than wire messages.
			if tc.Action != "validate" {
				continue
			}
			t.Run(tc.Name, func(t *testing.T) {
				var p a2ui.Processor
				var basic map[string]any
				if err := json.Unmarshal(a2ui.BasicCatalogDocument(), &basic); err != nil {
					t.Fatal(err)
				}

				for _, doc := range tc.Catalogs {
					// Supply the wire discriminator and common schema aliases used by the adapter fixture.
					for name, schema := range doc["components"].(map[string]any) {
						schema.(map[string]any)["properties"].(map[string]any)["component"] = map[string]string{"const": name}
					}
					doc["$defs"] = map[string]any{
						"DynamicString": map[string]string{"$ref": "https://a2ui.org/specification/v1_0/common_types.json#/$defs/DynamicString"},
					}
				}
				for _, doc := range append(tc.Catalogs, basic) {
					raw, err := json.Marshal(doc)
					if err != nil {
						t.Fatal(err)
					}
					catalog, err := a2ui.ParseCatalog(raw)
					if err != nil {
						t.Fatal(err)
					}
					p.RegisterCatalog(catalog)
				}
				for _, step := range tc.Steps {
					// This suite spells component catalog IDs using the
					// adapter field; A2UI 1.0 wire messages use @catalogId.
					for _, msg := range step.Payload {
						create, _ := msg["createSurface"].(map[string]any)
						if create["catalogId"] == "basic" {
							create["catalogId"] = a2ui.BasicCatalog
						}
						components, _ := create["components"].([]any)
						for _, item := range components {
							node := item.(map[string]any)
							if call, ok := node["text"].(map[string]any); ok && call["catalogId"] == "basic" {
								call["catalogId"] = a2ui.BasicCatalog
							}
							if id, ok := node["catalogId"]; ok {
								if id == "basic" {
									id = a2ui.BasicCatalog
								}
								node["@catalogId"] = id
								delete(node, "catalogId")
							}
						}
					}
					raw, err := json.Marshal(step.Payload)
					if err != nil {
						t.Fatal(err)
					}
					messages, err := a2ui.DecodeBatch(raw)
					if err != nil {
						t.Fatal(err)
					}
					if err := p.ApplyBatch(messages); err != nil {
						t.Fatal(err)
					}
					if step.ExpectError != nil {
						surface := p.Surface(messages[0].Create.ID)
						node, _ := surface.Component(a2ui.Root)
						if _, err := surface.Resolve(node.Props["text"], ""); err == nil || !strings.Contains(err.Error(), "@index") {
							t.Fatalf("expected a scope error, got %v", err)
						}
					}
				}
				for id, expected := range tc.Expect.Surfaces {
					surface := p.Surface(id)
					if surface == nil {
						t.Fatalf("missing surface %q", id)
					}
					tester := ui.NewTester(func(c *ui.Context) {
						a2uimygo.View(c, surface, a2uimygo.Options{
							Components: map[string]map[string]a2uimygo.ComponentView{

								"app-cat": {"Text": func(c *ui.Context, n a2uimygo.Node) {
									ui.Text(c, a2ui.Text(n.Property("text")))
								}},
							},
						})
					}, 640, 400)
					tester.Frame()
					for component, want := range expected.Components {
						if _, found := tester.Find(want.Text); !found {
							t.Errorf("component %s did not render %q; texts: %v", component, want.Text, tester.Texts())
						}
					}
				}
			})
		}
	}
}
