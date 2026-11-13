package a2uiagent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"

	"gopkg.in/yaml.v3"
)

type officialPart struct {
	Text  string
	A2UI  []map[string]any
	Error map[string]any
}

type officialCase struct {
	Action string
	Name   string
	Input  string
	Args   struct {
		Format             a2uiagent.Format
		Catalog            any
		Catalogs           []any
		Wrapped            *bool
		ProgressiveKeys    []string `yaml:"progressive_keys"`
		AfterRejectedBlock string   `yaml:"after_rejected_block"`
	}
	Steps []struct {
		Input       string
		Expect      []officialPart
		ExpectError map[string]any `yaml:"expect_error"`
	}
	Messages        []map[string]any
	ExpectContains  []string `yaml:"expect_contains"`
	ExpectAbsent    []string `yaml:"expect_absent"`
	ExpectRoundTrip bool     `yaml:"expect_round_trip"`
	Expect          []map[string]any
	ExpectError     map[string]any `yaml:"expect_error"`
	ExpectPresent   []any          `yaml:"expect_present"`
}

func fixtures(t *testing.T, suite string) []officialCase {
	t.Helper()
	data, err := os.ReadFile("testdata/official/" + suite + ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []officialCase
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func fixtureCatalogs(t *testing.T, tc officialCase) []*a2ui.Catalog {
	t.Helper()
	if len(tc.Args.Catalogs) == 0 {
		return []*a2ui.Catalog{catalogFor(t, tc.Args.Catalog)}
	}
	catalogs := make([]*a2ui.Catalog, 0, len(tc.Args.Catalogs))
	for _, item := range tc.Args.Catalogs {
		catalogs = append(catalogs, catalogFor(t, item))
	}
	return catalogs
}

func catalogFor(t *testing.T, name any) *a2ui.Catalog {
	t.Helper()
	var data []byte
	var err error
	switch value := name.(type) {
	case string:
		if strings.Contains(value, "v0_") || strings.Contains(value, "v09") || strings.Contains(value, "v08") {
			t.Skip("legacy protocol")
		}
		data, err = os.ReadFile("testdata/official/" + filepath.Base(value))
	case map[string]any:
		data, err = json.Marshal(value)
	default:
		t.Fatal("fixture is missing a catalog")
	}
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := a2ui.ParseCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func jsonEqual(a, b any) bool {
	var first, second any
	one, _ := json.Marshal(a)
	two, _ := json.Marshal(b)
	_ = json.Unmarshal(one, &first)
	_ = json.Unmarshal(two, &second)
	return reflect.DeepEqual(first, second)
}

func TestOfficialDirectJSONStream(t *testing.T) {
	testOfficialStream(t, "response_streaming", a2uiagent.DirectJSON)
}

func TestOfficialExpressStream(t *testing.T) {
	testOfficialStream(t, "express_streaming", a2uiagent.Express)
}

func testOfficialStream(t *testing.T, suite string, format a2uiagent.Format) {
	t.Helper()
	for _, tc := range fixtures(t, suite) {
		t.Run(tc.Name, func(t *testing.T) {
			var p a2ui.Processor
			catalogs := fixtureCatalogs(t, tc)
			for _, catalog := range catalogs {
				p.RegisterCatalog(catalog)
			}
			stream := a2uiagent.Stream{State: &p, Format: format, Catalogs: catalogs, Unwrapped: tc.Args.Wrapped != nil && !*tc.Args.Wrapped, ProgressiveFields: tc.Args.ProgressiveKeys, Continue: tc.Args.AfterRejectedBlock == "continue"}
			for i, step := range tc.Steps {
				parts, err := stream.Push(step.Input)
				if step.ExpectError != nil {
					if err == nil {
						t.Fatalf("step %d expected an error", i+1)
					}
					return
				}
				if err != nil {
					t.Fatalf("step %d: %v", i+1, err)
				}
				if len(parts) != len(step.Expect) {
					t.Fatalf("step %d got %d parts, want %d (%+v)", i+1, len(parts), len(step.Expect), parts)
				}
				for j, want := range step.Expect {
					got := parts[j]
					if got.Text != want.Text || (got.Err != nil) != (want.Error != nil) || !jsonEqual(got.Messages, want.A2UI) {
						t.Fatalf("step %d part %d: got %+v, want %+v", i+1, j+1, got, want)
					}
				}
			}
		})
	}
}

func TestOfficialExpressCompiler(t *testing.T) {
	for _, tc := range fixtures(t, "express_compiler") {
		t.Run(tc.Name, func(t *testing.T) {
			if strings.Contains(tc.Name, "v09") || strings.Contains(tc.Name, "v08") {
				t.Skip("legacy protocol")
			}
			messages, err := a2uiagent.CompileExpressCatalogs(tc.Input, fixtureCatalogs(t, tc), nil)
			if tc.ExpectError != nil {
				if err == nil {
					t.Fatal("expected compiler error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := json.Marshal(messages)
			if len(tc.ExpectPresent) > 0 {
				var decoded []map[string]any
				if err := json.Unmarshal(actual, &decoded); err != nil {
					t.Fatal(err)
				}
				for _, message := range decoded {
					create, _ := message["createSurface"].(map[string]any)
					components, _ := create["components"].([]any)
					for _, component := range components {
						obj, _ := component.(map[string]any)
						checks, _ := obj["@checks"].([]any)
						for _, check := range checks {
							rule, _ := check.(map[string]any)
							if message, _ := rule["message"].(string); message == "" {
								t.Fatal("missing generated validation message")
							}
							delete(rule, "message")
						}
					}
				}
				if jsonEqual(decoded, tc.Expect) {
					return
				}
			}
			if !jsonEqual(messages, tc.Expect) {
				want, _ := json.Marshal(tc.Expect)
				t.Fatalf("messages = %s, want %s", actual, want)
			}
		})
	}
}

func TestOfficialDirectJSONCompiler(t *testing.T) {
	for _, tc := range fixtures(t, "direct_json_compiler") {
		t.Run(tc.Name, func(t *testing.T) {
			messages, err := a2uiagent.CompileDirectJSON(tc.Input, fixtureCatalogs(t, tc))
			if tc.ExpectError != nil {
				if err == nil {
					t.Fatal("expected compiler error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(messages, tc.Expect) {
				actual, _ := json.Marshal(messages)
				t.Fatalf("compiled %s, expected %v", actual, tc.Expect)
			}
		})
	}
}

func TestOfficialDirectJSONDecompiler(t *testing.T) {
	for _, tc := range fixtures(t, "direct_json_decompiler") {
		t.Run(tc.Name, func(t *testing.T) {
			catalogs := fixtureCatalogs(t, tc)
			raw, err := json.Marshal(tc.Messages)
			if err != nil {
				t.Fatal(err)
			}
			messages, err := a2ui.DecodeBatch(raw)
			if err != nil {
				t.Fatal(err)
			}
			source, err := a2uiagent.Decompile(a2uiagent.DirectJSON, messages)
			if err != nil {
				t.Fatal(err)
			}
			for _, fragment := range tc.ExpectContains {
				if !strings.Contains(source, fragment) {
					t.Errorf("decompiled source does not contain %q", fragment)
				}
			}
			if tc.ExpectRoundTrip {
				recompiled, err := a2uiagent.CompileDirectJSON(source, catalogs)
				if err != nil {
					t.Fatal(err)
				}
				if !jsonEqual(recompiled, tc.Messages) {
					t.Fatalf("round trip changed messages: %s", source)
				}
			}
		})
	}
}

func TestOfficialRendererCapabilities(t *testing.T) {
	data, err := os.ReadFile("testdata/official/renderer_capabilities.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string
		Args struct {
			Version  string
			Metadata map[string]any
		}
		Expect      any
		ExpectError any `yaml:"expect_error"`
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Args.Version != a2ui.Version {
				t.Skip("legacy protocol")
			}
			raw, err := json.Marshal(tc.Args.Metadata)
			if err != nil {
				t.Fatal(err)
			}
			var metadata map[string]json.RawMessage
			if err := json.Unmarshal(raw, &metadata); err != nil {
				t.Fatal(err)
			}
			actual, err := a2uiagent.ReadRendererCapabilities(metadata)
			if (err != nil) != (tc.ExpectError != nil) {
				t.Fatalf("error = %v, expected %v", err, tc.ExpectError)
			}
			if err == nil && !jsonEqual(actual, tc.Expect) {
				t.Fatalf("got %+v, want %+v", actual, tc.Expect)
			}
		})
	}
}
