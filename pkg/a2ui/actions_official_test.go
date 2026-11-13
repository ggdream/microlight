package a2ui

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestOfficialActionDispatch(t *testing.T) {
	data, err := os.ReadFile("testdata/official/actions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name           string
		Version        string `yaml:"protocolVersion"`
		Surface        string `yaml:"surfaceId"`
		Component      string `yaml:"sourceComponentId"`
		Scope          string
		Model          map[string]any `yaml:"dataModel"`
		Action         map[string]any `yaml:"actionPayload"`
		Metadata       *Metadata
		UserActivation bool           `yaml:"userActivation"`
		Dispatched     map[string]any `yaml:"expectDispatched"`
		ExpectData     map[string]any `yaml:"expectDataModel"`
		ExpectError    any            `yaml:"expectError"`
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Version != Version {
				t.Skip("fixture uses pre-1.0 action semantics")
			}
			node, err := NewComponent(tc.Component, "Button", map[string]any{"action": tc.Action})
			if err != nil {
				t.Fatal(err)
			}
			surface := &Surface{
				id:         tc.Surface,
				components: map[string]Component{tc.Component: node},
				data:       Model{root: tc.Model},
			}
			surface.data.EvaluateFunction = func(call FunctionCall, scope string) (any, error) {
				switch call.Name {
				case "incrementCounter":
					path := Text(call.Args["path"])
					previous, _ := surface.data.Get(path)
					return nil, surface.data.Set(path, Number(previous)+Number(call.Args["step"]))
				case "failingAction":
					return nil, fmt.Errorf("fixture action failed")
				default:
					return surface.data.basicFunction(call, scope)
				}
			}
			var result *RendererMessage
			if tc.Action["functionCall"] != nil {
				var open func(string) error
				if tc.UserActivation {
					open = func(string) error { return nil }
				}
				result, err = surface.Activate(tc.Component, tc.Scope, time.Now(), open)
			} else {
				var event RendererMessage
				event, err = surface.ActionWithMetadata(tc.Component, tc.Scope, time.Now(), tc.Metadata)
				if err == nil {
					result = &event
				}
			}
			if (err != nil) != (tc.ExpectError != nil) {
				t.Fatalf("dispatch error %v, expected %v", err, tc.ExpectError)
			}
			if err != nil {
				return
			}
			if (result == nil) != (tc.Dispatched == nil) {
				t.Fatalf("dispatched %v, want %v", result, tc.Dispatched)
			}
			if result != nil {
				raw, _ := json.Marshal(result.Action)
				var actual map[string]any
				_ = json.Unmarshal(raw, &actual)
				if tc.Dispatched["timestampIso8601"] == true {
					_, parseErr := time.Parse(time.RFC3339Nano, Text(actual["timestamp"]))
					actual["timestampIso8601"] = parseErr == nil
				}
				for key, want := range tc.Dispatched {
					raw, _ := json.Marshal(want)
					var expected any
					_ = json.Unmarshal(raw, &expected)
					if !reflect.DeepEqual(actual[key], expected) {
						t.Errorf("action %s = %#v, want %#v", key, actual[key], expected)
					}
				}
			}
			if tc.ExpectData != nil {
				actual, _ := surface.Data().Get("")
				a, _ := json.Marshal(actual)
				b, _ := json.Marshal(tc.ExpectData)
				if string(a) != string(b) {
					t.Errorf("data = %s, want %s", a, b)
				}
			}
		})
	}
}
