package a2ui

import (
	"encoding/json"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOfficialCommonTypes(t *testing.T) {
	data, err := os.ReadFile("testdata/official/common_types.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name       string
		Action     string
		Version    string `yaml:"protocolVersion"`
		Definition string
		Steps      []struct {
			Value       any
			ExpectError any `yaml:"expectError"`
		}
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Version != Version {
				t.Skip("legacy protocol")
			}
			if tc.Action == "common_types_schema" {
				t.Skip("Go uses the published schema directly; this case tests schema generation from SDK models")
			}
			schema, err := compileSchema([]byte(`{"$ref":"common_types.json#/$defs/`+tc.Definition+`"}`), schemaBase+"test-common-types.json")
			if err != nil {
				t.Fatal(err)
			}
			for _, step := range tc.Steps {
				raw, err := json.Marshal(step.Value)
				if err != nil {
					t.Fatal(err)
				}
				err = validateJSON(schema, raw)
				if (err != nil) != (step.ExpectError != nil) {
					t.Errorf("value %s: error %v, want %v", raw, err, step.ExpectError)
				}
			}
		})
	}
}
