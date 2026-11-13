package a2uimygo_test

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
	"gopkg.in/yaml.v3"
)

type adapterNode struct {
	ID               string
	Type             string
	Text             string
	BadgeText        string   `yaml:"badgeText"`
	IsValid          *bool    `yaml:"isValid"`
	ValidationErrors []string `yaml:"validationErrors"`
	Hints            []string
	Children         []adapterNode
}

func TestOfficialMyGoAdapter(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/adapter_conformance.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string
		Catalogs []map[string]any
		Steps    []struct {
			Op            string
			Messages      []map[string]any
			Target        string           `yaml:"targetNode"`
			Input         any              `yaml:"inputValue"`
			Tree          *adapterNode     `yaml:"expectViewTree"`
			Data          map[string]any   `yaml:"expectDataModel"`
			Actions       []map[string]any `yaml:"expectActions"`
			Accessibility map[string]struct {
				Label       string
				Description string
				Role        string
				Hidden      bool
				Live        string
			} `yaml:"expectAccessibilityTree"`
			SurfaceRemoved bool `yaml:"expectSurfaceRemoved"`
		}
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{
		"action_execution_event_vs_local_function_call":          "fixture-local mutation needs a catalog evaluator",
		"user_activation_gated_function_gesture_vs_programmatic": "programmatic dispatch diagnostics are not asserted",
		"v10_theme_dropped_in_permissive_mode_renders_like_none": "requires a permissive envelope mode",
		"fallback_states_unknown_type_and_cyclic_diagnostics":    "requires renderable diagnostic nodes",
		"teardown_unmount_releases_subscriptions":                "requires subscription lifecycle instrumentation",
		"removed_components_release_subscriptions":               "requires subscription lifecycle instrumentation",
		"two_way_binding_inputs_checkbox_choice_slider_datetime": "control-specific input steps need adapter support",
		"media_components_bind_url_and_name":                     "requires media widget state assertions",
		"agent_function_action_failure_reports_expression_error": "requires agent RPC and renderer error assertions",
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if reason := reasons[tc.Name]; reason != "" {
				t.Skip(reason)
			}
			var p a2ui.Processor
			views := make(map[string]map[string]a2uimygo.ComponentView)
			for _, doc := range tc.Catalogs {
				raw, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				catalog, err := a2ui.ParseCatalog(raw)
				if err != nil {
					t.Fatal(err)
				}
				p.RegisterCatalog(catalog)
				views[catalog.ID] = map[string]a2uimygo.ComponentView{
					"StatusBadge": func(c *ui.Context, node a2uimygo.Node) {
						ui.Text(c, a2ui.Text(node.Property("badgeText")))
					},
				}
			}
			var actions []a2ui.ActionEvent
			id := "adapter_surface"
			previousTexts := make(map[string]bool)
			tester := ui.NewTester(func(c *ui.Context) {
				a2uimygo.View(c, p.Surface(id), a2uimygo.Options{
					Components: views,
					OnAction:   func(message a2ui.RendererMessage) { actions = append(actions, *message.Action) },
				})
			}, 760, 640)
			tester.UseScreenReader()
			for _, step := range tc.Steps {
				actions = nil
				surface := p.Surface(id)
				switch step.Op {
				case "process_messages":
					// The adapter fixtures still use unprefixed component
					// mixin aliases. Translate those to the 1.0 wire spelling.
					for _, message := range step.Messages {
						for _, key := range []string{"createSurface", "updateComponents"} {
							body, _ := message[key].(map[string]any)
							nodes, _ := body["components"].([]any)
							for _, raw := range nodes {
								node := raw.(map[string]any)
								for _, alias := range []string{"checks", "accessibility"} {
									if value, ok := node[alias]; ok {
										node["@"+alias] = value
										delete(node, alias)
									}
								}
							}
						}
					}
					raw, err := json.Marshal(step.Messages)
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
				case "user_input", "user_gesture":
					node, ok := surface.Component(step.Target)
					if !ok {
						t.Fatalf("unknown target %s", step.Target)
					}
					label := adapterLabel(surface, node)
					if node.Type == "Tabs" {
						var tabs []struct{ Title json.RawMessage }
						if err := json.Unmarshal(node.Props["tabs"], &tabs); err != nil {
							t.Fatal(err)
						}
						label = surface.String(tabs[step.Input.(int)].Title, "")
					}
					if err := tester.Click(label); err != nil {
						t.Fatal(err)
					}
					if node.Type == "TextField" {
						tester.Command("selectAll")
						tester.Type(step.Input.(string))
					}
				default:
					t.Fatalf("unhandled adapter operation %q", step.Op)
				}
				tester.Frame()
				surface = p.Surface(id)
				if step.SurfaceRemoved && (surface != nil || len(tester.Texts()) != 0) {
					t.Fatal("deleted surface is still visible")
				}
				if step.Tree != nil {
					current := make(map[string]bool)
					assertAdapterNode(t, tester, surface, *step.Tree, current)
					for text := range previousTexts {
						if !current[text] {
							if _, found := tester.Find(text); found {
								t.Errorf("stale text %q remains visible", text)
							}
						}
					}
					previousTexts = current
				}
				if step.Data != nil {
					actual, _ := surface.Data().Get("")
					adapterEqual(t, actual, step.Data)
				}
				if step.Actions != nil {
					actual := make([]a2ui.ActionEvent, len(actions))
					copy(actual, actions)
					raw, _ := json.Marshal(actual)
					var fields []map[string]any
					_ = json.Unmarshal(raw, &fields)
					for _, event := range fields {
						delete(event, "timestamp")
					}
					adapterEqual(t, fields, step.Actions)
				}
				for id, want := range step.Accessibility {
					node, _ := surface.Component(id)
					label := want.Label
					if label == "" {
						label = adapterLabel(surface, node)
					}
					info, ok := tester.Access(label)
					if want.Hidden {
						if ok {
							t.Errorf("%s remains in the accessibility tree", id)
						}
						continue
					}
					if !ok || (want.Description != "" && info.Description != want.Description) || (want.Role == "button" && info.Role != ui.RoleButton) {
						t.Errorf("accessibility node %s = %+v (present %v)", id, info, ok)
					}
				}
			}
		})
	}
}

func adapterEqual(t *testing.T, got, want any) {
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

func adapterLabel(surface *a2ui.Surface, node a2ui.Component) string {
	if node.Type == "Button" {
		child, _ := surface.Component(surface.String(node.Props["child"], ""))
		return surface.String(child.Props["text"], "")
	}
	if label := surface.String(node.Props["label"], ""); label != "" {
		return label
	}
	return surface.String(node.Props["text"], "")
}

func assertAdapterNode(t *testing.T, tester *ui.Tester, surface *a2ui.Surface, want adapterNode, texts map[string]bool) {
	t.Helper()
	node, exists := surface.Component(want.ID)
	if want.Type != "" && (!exists || node.Type != want.Type) {
		t.Fatalf("component %s = %q, want %q", want.ID, node.Type, want.Type)
	}
	for _, text := range []string{want.Text, want.BadgeText} {
		if text == "" {
			continue
		}
		texts[text] = true
		if _, found := tester.Find(text); !found {
			if _, visible := tester.Find(strings.TrimLeft(text, "# ")); !visible {
				t.Errorf("text %q missing from %v", text, tester.Texts())
			}
		}
	}
	if want.IsValid != nil {
		label := adapterLabel(surface, node)
		access, ok := tester.Access(label)
		if !ok || access.Invalid == *want.IsValid {
			t.Errorf("accessibility validity for %s = %v (present %v)", want.ID, !access.Invalid, ok)
		}
	}
	hints := append(append([]string{}, want.ValidationErrors...), want.Hints...)
	sort.Strings(hints)
	for _, hint := range hints {
		if _, found := tester.Find(hint); !found {
			t.Errorf("missing validation feedback %q", hint)
		}
	}
	for _, child := range want.Children {
		assertAdapterNode(t, tester, surface, child, texts)
	}
}
