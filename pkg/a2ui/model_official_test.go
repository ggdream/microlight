package a2ui_test

import (
	"os"
	"sort"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"gopkg.in/yaml.v3"
)

func TestOfficialDataModel(t *testing.T) {
	data, err := os.ReadFile("testdata/official/data_model.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string
		Initial any
		Watch   []string
		Steps   []map[string]any
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var model a2ui.Model
			if err := model.Set("", tc.Initial); err != nil {
				t.Fatal(err)
			}
			var notified []string
			var subscriptions []func()
			for _, path := range tc.Watch {
				stop, err := model.Subscribe(path, func(any) { notified = append(notified, path) })
				if err != nil {
					t.Fatal(err)
				}
				subscriptions = append(subscriptions, stop)
				t.Cleanup(stop)
			}
			var callerValue any
			for _, step := range tc.Steps {
				notified = make([]string, 0)
				path, _ := step["path"].(string)
				var err error
				switch step["op"] {
				case "get":
					var value any
					var found bool
					value, found, err = model.Lookup(path)
					if step["expect_absent"] == true && found && value != nil {
						t.Errorf("%q unexpectedly exists: %v", path, value)
					}
					if expect, ok := step["expect"]; ok {
						compareJSON(t, value, expect)
					}
					switch step["expect_type"] {
					case "list":
						if _, ok := value.([]any); !ok {
							t.Errorf("%s is not a list: %T", path, value)
						}
					case "object":
						if _, ok := value.(map[string]any); !ok {
							t.Errorf("%s is not an object: %T", path, value)
						}
					}
				case "set":
					callerValue = step["value"]
					err = model.Set(path, callerValue)
				case "delete":
					err = model.Set(path, nil)
				case "dispose":
					for _, stop := range subscriptions {
						stop()
					}
				case "mutate_input":
					fixtureMap(callerValue)[path[1:]] = step["value"]
				default:
					t.Fatalf("unknown data model operation %v", step["op"])
				}
				if (err != nil) != (step["expect_error"] != nil) {
					t.Fatalf("%s %q: error %v, expected %v", step["op"], path, err, step["expect_error"])
				}
				if values, ok := step["expect_notified"].([]any); ok {
					want := make([]string, len(values))
					for i, path := range values {
						want[i] = path.(string)
					}
					sort.Strings(want)
					sort.Strings(notified)
					compareJSON(t, notified, want)
				}
				for path, want := range fixtureMap(step["expect_values"]) {
					got, _ := model.Get(path)
					compareJSON(t, got, want)
				}
			}
		})
	}
}
