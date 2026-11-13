package a2ui_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"

	"gopkg.in/yaml.v3"
)

type stateCase struct {
	Name  string
	Steps []struct {
		Payload     []map[string]any
		ExpectError map[string]any `yaml:"expectError"`
	}
	Expect      yaml.Node
	Function    string
	Protocol    string `yaml:"protocolVersion"`
	Catalog     string `yaml:"catalogId"`
	Surface     map[string]any
	Args        map[string]any
	ExpectError map[string]any `yaml:"expectError"`
}

type stateExpectation struct {
	Surfaces map[string]struct {
		DataModel  map[string]any `yaml:"dataModel"`
		Components map[string]map[string]any
	}
}

func TestOfficialStateAndFunctions(t *testing.T) {
	for _, suite := range []string{"data_model_pointers", "data_deletion", "functions"} {
		raw, err := os.ReadFile("testdata/official/" + suite + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		var cases []stateCase
		if err := yaml.Unmarshal(raw, &cases); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			t.Run(tc.Name, func(t *testing.T) {
				if tc.Function != "" {
					if strings.Contains(tc.Name, "_v09_") || tc.Protocol == "v0.9" {
						t.Skip("legacy protocol")
					}
					if tc.Function == "openUrl" && tc.ExpectError == nil {
						t.Skip("requires an explicit host action; exercised by renderer activation tests")
					}
					var model a2ui.Model
					call := map[string]any{"@call": tc.Function, "args": tc.Args}
					if tc.Catalog != "" {
						call["catalogId"] = tc.Catalog
					}
					if tc.Surface != nil {
						var p a2ui.Processor
						basic, err := a2ui.ParseCatalog(a2ui.BasicCatalogDocument())
						if err != nil {
							t.Fatal(err)
						}
						p.RegisterCatalog(basic)
						id, _ := tc.Surface["catalogId"].(string)
						if id != "" && id != a2ui.BasicCatalog {
							registerFixtureCatalogs(t, &p, protocolCase{Catalog: map[string]any{
								"catalogId":  id,
								"components": map[string]any{"Text": map[string]any{"type": "object"}},
							}})
						}
						if err := p.Apply(a2ui.AgentMessage{Version: a2ui.Version, Create: &a2ui.CreateSurface{SurfaceRef: a2ui.SurfaceRef{ID: "test"}, Catalog: id}}); err != nil {
							t.Fatal(err)
						}
						model = *p.Surface("test").Data()
					}
					value, err := model.Resolve(call, "")
					if tc.ExpectError != nil {
						if err == nil {
							t.Fatal("expected function error")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					var expected any
					if err := tc.Expect.Decode(&expected); err != nil {
						t.Fatal(err)
					}
					if _, ok := expected.(bool); ok {
						value = a2ui.Boolean(value)
					}
					if result, ok := value.(a2ui.ValidationResult); ok {
						if object, ok := expected.(map[string]any); ok {
							value = map[string]any{"valid": result.Valid}
							if _, present := object["message"]; present {
								value.(map[string]any)["message"] = result.Message
							}
						}
					}
					if !reflect.DeepEqual(value, expected) {
						t.Fatalf("value = %#v, want %#v", value, expected)
					}
					return
				}
				var processor a2ui.Processor
				var catalog map[string]any
				_ = json.Unmarshal(a2ui.BasicCatalogDocument(), &catalog)
				catalog["catalogId"] = "basic"
				data, _ := json.Marshal(catalog)
				basic, err := a2ui.ParseCatalog(data)
				if err != nil {
					t.Fatal(err)
				}
				processor.RegisterCatalog(basic)
				for _, step := range tc.Steps {
					data, err := json.Marshal(step.Payload)
					if err != nil {
						t.Fatal(err)
					}
					messages, err := a2ui.DecodeBatch(data)
					if err != nil {
						t.Fatal(err)
					}
					err = processor.ApplyBatch(messages)
					if step.ExpectError != nil {
						if err == nil {
							t.Fatal("expected data error")
						}
						continue
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				var expected stateExpectation
				if err := tc.Expect.Decode(&expected); err != nil {
					t.Fatal(err)
				}
				for id, want := range expected.Surfaces {
					surface := processor.Surface(id)
					if surface == nil {
						t.Fatalf("missing surface %q", id)
					}
					if want.DataModel != nil {
						value, _ := surface.Data().Get("")
						got, _ := json.Marshal(value)
						expected, _ := json.Marshal(want.DataModel)
						if string(got) != string(expected) {
							t.Errorf("data = %s, want %s", got, expected)
						}
					}
					for key, props := range want.Components {
						component, ok := surface.Component(key)
						if !ok {
							t.Fatalf("missing component %q", key)
						}
						for prop, expected := range props {
							if prop == "component" {
								if component.Type != expected {
									t.Errorf("type = %q, want %v", component.Type, expected)
								}
								continue
							}
							actual, err := surface.Resolve(component.Props[prop], "")
							if err != nil || !reflect.DeepEqual(actual, expected) {
								t.Errorf("%s.%s = %#v (%v), want %#v", key, prop, actual, err, expected)
							}
						}
					}
				}
			})
		}
	}
}
