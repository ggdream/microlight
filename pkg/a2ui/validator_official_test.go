package a2ui_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"gopkg.in/yaml.v3"
)

type protocolStep struct {
	Messages    any
	Payload     any
	Expect      yaml.Node
	ExpectError any             `yaml:"expectError"`
	Config      map[string]bool `yaml:"validationConfig"`
}

type protocolCase struct {
	protocolStep `yaml:",inline"`
	Name         string
	Action       string
	Args         struct {
		Version string
		Inline  bool `yaml:"includeInlineCatalogs"`
	}

	StrictMode bool `yaml:"strictMode"`
	Catalog    map[string]any
	Catalogs   []map[string]any
	Steps      []protocolStep
}

func TestOfficialProtocolValidation(t *testing.T) {
	for _, suite := range []string{"validator_v1_0", "composition_constraints", "reserved_keys", "validation_result", "message_processor_v1_0"} {
		raw, err := os.ReadFile("testdata/official/" + suite + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		var cases []protocolCase
		if err := yaml.Unmarshal(raw, &cases); err != nil {
			t.Fatal(err)
		}
		t.Run(suite, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.Name, func(t *testing.T) {
					switch tc.Name {
					case "test_v091_version_string_compatibility",
						"test_v10_get_renderer_data_model_excludes_surface_without_default_from_v09",
						"test_v10_get_renderer_data_model_filters_by_surface_protocol_version":
						t.Skip("mixed or legacy protocol version")
					}
					var p a2ui.Processor
					p.AllowUndeclaredFunctions(suite == "message_processor_v1_0" && !tc.StrictMode)
					registerFixtureCatalogs(t, &p, tc)
					if tc.Action == "get_renderer_capabilities" {
						var want any
						if err := tc.Expect.Decode(&want); err != nil {
							t.Fatal(err)
						}
						got := p.Capabilities(tc.Args.Inline)
						compareJSON(t, got, want)
						return
					}
					steps := tc.Steps
					if tc.Messages != nil || tc.Payload != nil {
						header := tc.protocolStep
						header.Config = nil
						steps = append([]protocolStep{header}, steps...)
					}
					for i, step := range steps {
						if i == len(steps)-1 && step.ExpectError == nil {
							step.ExpectError = tc.ExpectError
						}
						payload := step.Messages
						if payload == nil {
							payload = step.Payload
						}
						if payload != nil {
							raw, err := json.Marshal(payload)
							if err != nil {
								t.Fatal(err)
							}
							messages, err := a2ui.DecodeBatch(raw)
							candidate := p.Fork()
							if err == nil {
								err = candidate.ApplyBatch(messages)
							}
							if err == nil {
								switch {
								case step.Config != nil:
									err = validateFixtureGraph(candidate, messages, step.Config["allowOrphanComponents"], step.Config["allowDanglingReferences"])
								case tc.StrictMode || (tc.Config != nil && !tc.Config["allowOrphanComponents"] && len(tc.Config) != 0):
									err = validateFixtureGraph(candidate, messages, false, false)
								}
							}
							if step.ExpectError != nil {
								if err == nil {
									t.Fatal("expected validation error")
								}
							} else if err != nil {
								t.Fatal(err)
							} else {
								p = *candidate
							}
						}
						assertProtocolState(t, &p, step.Expect)
					}
					if tc.Action == "get_renderer_data_model" {
						var want any
						if err := tc.Expect.Decode(&want); err != nil {
							t.Fatal(err)
						}
						compareJSON(t, p.DataModel(), want)
					} else {
						assertProtocolState(t, &p, tc.Expect)
					}
				})
			}
		})
	}
}

func registerFixtureCatalogs(t *testing.T, p *a2ui.Processor, tc protocolCase) {
	t.Helper()
	var basic map[string]any
	if err := json.Unmarshal(a2ui.BasicCatalogDocument(), &basic); err != nil {
		t.Fatal(err)
	}
	var docs []map[string]any
	aliases := []string{"basic", "test-catalog-v10", "https://a2ui.org/catalogs/basic/v1/catalog.json"}
	if tc.Action == "get_renderer_capabilities" {
		aliases = nil
	}
	for _, alias := range aliases {
		copy := make(map[string]any)
		for key, val := range basic {
			copy[key] = val
		}
		copy["catalogId"] = alias
		docs = append(docs, copy)
	}
	if schema := fixtureMap(tc.Catalog["catalogSchema"]); schema != nil {
		tc.Catalog = schema
	}
	for _, doc := range append(tc.Catalogs, tc.Catalog) {
		if doc == nil || doc["catalogId"] == nil {
			continue
		}
		// The Dart fixture loader supplies the discriminator for inline
		// component definitions that only declare their own properties.
		if version, ok := doc["protocolVersion"].(string); ok && version != a2ui.Version {
			continue
		}
		if tc.Name == "test_v10_component_catalog_override" {
			switch doc["catalogId"] {
			case "main-catalog":
				doc["$defs"] = basic["$defs"]
				doc["components"] = basic["components"]
				doc["functions"] = basic["functions"]
			case "charts-catalog":
				doc["components"] = map[string]any{"PieChart": map[string]any{
					"type": "object", "properties": map[string]any{"data": map[string]any{"type": "array"}}, "required": []string{"data"},
				}}
			}
		}
		if doc["components"] == nil && doc["functions"] == nil {
			doc["components"] = map[string]any{}
		}
		if doc["protocolVersion"] == nil {
			doc["protocolVersion"] = a2ui.Version
		}
		defs := fixtureMap(doc["$defs"])
		if defs == nil {
			defs = make(map[string]any)
			doc["$defs"] = defs
		}
		for _, name := range []string{"ChildList", "ComponentId", "DynamicString", "DynamicValue"} {
			if tc.Action != "get_renderer_capabilities" && defs[name] == nil {
				defs[name] = map[string]string{"$ref": "common_types.json#/$defs/" + name}
			}
		}
		for name, raw := range fixtureMap(doc["components"]) {
			schema := fixtureMap(raw)
			props := fixtureMap(schema["properties"])
			if props == nil {
				props = make(map[string]any)
				schema["properties"] = props
			}
			if props["component"] == nil {
				props["component"] = map[string]any{"const": name}
			}
		}
		docs = append(docs, doc)
	}
	for _, doc := range docs {
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
}

func fixtureMap(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func validateFixtureGraph(p *a2ui.Processor, messages []a2ui.AgentMessage, allowOrphans, allowDangling bool) error {
	for _, message := range messages {
		if message.Create != nil && len(message.Create.Components) != 0 {
			if err := p.Surface(message.Create.ID).ValidateTopology(allowOrphans, allowDangling); err != nil {
				return err
			}
		}
		if message.Components != nil {
			surface := p.Surface(message.Components.ID)
			if surface != nil {
				if err := surface.ValidateTopology(allowOrphans, allowDangling); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func assertProtocolState(t *testing.T, p *a2ui.Processor, expected yaml.Node) {
	t.Helper()
	if expected.Kind == 0 {
		return
	}
	var state struct {
		Surfaces map[string]struct {
			Components        any
			Exists            *bool
			DataModel         any   `yaml:"dataModel"`
			SendDataModel     *bool `yaml:"sendDataModel"`
			Metadata          any
			Catalog           string                             `yaml:"catalogId"`
			IsValid           map[string]bool                    `yaml:"isValid"`
			ValidationErrors  map[string][]string                `yaml:"validationErrors"`
			ValidationResults map[string][]a2ui.ValidationResult `yaml:"validationResults"`
			ValidationResult  map[string]a2ui.ValidationResult   `yaml:"validationResult"`
		}
	}
	if err := expected.Decode(&state); err != nil {
		t.Fatal(err)
	}
	for id, want := range state.Surfaces {
		surface := p.Surface(id)
		if want.Exists != nil {
			compareJSON(t, surface != nil, *want.Exists)
			if !*want.Exists {
				continue
			}
		}
		if surface == nil {
			t.Fatalf("missing surface %q", id)
		}
		if want.DataModel != nil {
			value, _ := surface.Data().Get("")
			compareJSON(t, value, want.DataModel)
		}
		if want.SendDataModel != nil {
			compareJSON(t, surface.SendDataModel(), *want.SendDataModel)
		}
		if want.Metadata != nil {
			compareJSON(t, surface.Metadata(), want.Metadata)
		}
		if want.Catalog != "" {
			compareJSON(t, surface.Catalog(), want.Catalog)
		}
		for key, valid := range want.IsValid {
			node, _ := surface.Component(key)
			compareJSON(t, surface.IsValid(node, ""), valid)
		}
		for key, messages := range want.ValidationErrors {
			node, _ := surface.Component(key)
			actual := make([]string, 0)
			for _, result := range surface.Checks(node, "") {
				if result.Severity == "error" {
					actual = append(actual, result.Message)
				}
			}
			compareJSON(t, actual, messages)
		}
		for key, results := range want.ValidationResults {
			node, _ := surface.Component(key)
			compareJSON(t, append([]a2ui.ValidationResult{}, surface.Checks(node, "")...), results)
		}
		for key, result := range want.ValidationResult {
			node, _ := surface.Component(key)
			got := surface.Checks(node, "")
			if len(got) == 0 {
				t.Fatal("missing validation result")
			}
			if result.Severity == "" {
				got[0].Severity = ""
			}
			compareJSON(t, got[0], result)
		}
		nodes := make(map[string]map[string]any)
		switch components := want.Components.(type) {
		case []any:
			for _, node := range components {
				fields := fixtureMap(node)
				nodes[fields["id"].(string)] = fields
			}
		case map[string]any:
			for key, node := range components {
				nodes[key] = fixtureMap(node)
			}
		}
		for key, fields := range nodes {
			node, exists := surface.Component(key)
			if !exists {
				t.Fatalf("missing component %q", key)
			}
			for property, value := range fields {
				switch property {
				case "id":
					compareJSON(t, node.ID, value)
				case "component":
					compareJSON(t, node.Type, value)
				case "metadata", "catalogId", "@metadata", "@catalogId":
					if node.Props[property] == nil {
						property = "@" + property
					}
					if property == "@catalogId" && node.Props[property] == nil {
						compareJSON(t, node.Catalog, value)
						continue
					}
					var got any
					if raw := node.Props[property]; raw != nil {
						if err := json.Unmarshal(raw, &got); err != nil {
							t.Fatal(err)
						}
					}
					compareJSON(t, got, value)
				default:
					got, err := surface.Resolve(node.Props[property], "")
					if err != nil {
						t.Fatal(err)
					}
					compareJSON(t, got, value)
				}
			}
		}
	}
}

func TestOfficialDataContext(t *testing.T) {
	data, err := os.ReadFile("testdata/official/data_context.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string
		Args struct {
			Path    string
			Context string `yaml:"contextPath"`
		}
		Expect string
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			scope := tc.Args.Context
			if scope == "" {
				scope = "/"
			}
			compareJSON(t, a2ui.ResolvePath(scope, tc.Args.Path), tc.Expect)
		})
	}
}

func compareJSON(t *testing.T, got, want any) {
	t.Helper()
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	var x, y any
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	if !reflect.DeepEqual(x, y) {
		t.Fatalf("got %s, want %s", a, b)
	}
}
