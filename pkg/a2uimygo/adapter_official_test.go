package a2uimygo_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
	"gopkg.in/yaml.v3"
	"time"
)

type adapterNode struct {
	ID               string
	Type             string
	Text             string
	BadgeText        string `yaml:"badgeText"`
	URL              string
	Name             string
	State            string
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
		Name       string
		StrictMode *bool `yaml:"strictMode"`
		Catalogs   []map[string]any
		Steps      []struct {
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
			Errors         []struct {
				Code      string
				SurfaceID string `yaml:"surfaceId"`
			} `yaml:"expectErrors"`
		}
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var p a2ui.Processor
			p.AllowViewFallbacks(tc.Name == "fallback_states_unknown_type_and_cyclic_diagnostics")
			p.AllowUndeclaredFunctions(tc.Name == "action_execution_event_vs_local_function_call" || tc.Name == "agent_function_action_failure_reports_expression_error")
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
			var opened []string
			var remoteOptions *a2uimygo.AgentActionRPC
			remoteRequests := make(chan a2ui.RendererMessage, 1)
			var remoteErrors []a2ui.RendererMessage
			var callID string
			if tc.Name == "agent_function_action_failure_reports_expression_error" {
				rpc := &a2ui.RPC{Send: func(event a2ui.RendererMessage) error { remoteRequests <- event; return nil }}
				remoteOptions = &a2uimygo.AgentActionRPC{Transport: rpc, IsRemote: func(call a2ui.FunctionCall) bool { return call.Name == "lookupAccount" },
					EvaluateLocal: func(model *a2ui.Model, call a2ui.FunctionCall, _ string) (any, error) {
						path := a2ui.Text(call.Args["path"])
						before, _ := model.Get(path)
						return nil, model.Set(path, a2ui.Number(before)+a2ui.Number(call.Args["step"]))
					}}
			}
			diagnostics := make(map[string]string)
			id := "adapter_surface"
			mounted := true
			previousTexts := make(map[string]bool)
			tester := ui.NewTester(func(c *ui.Context) {
				if !mounted {
					return
				}
				a2uimygo.View(c, p.Surface(id), a2uimygo.Options{
					Components:      views,
					AgentRPC:        remoteOptions,
					OnRendererError: func(event a2ui.RendererMessage) { remoteErrors = append(remoteErrors, event) },
					OnError: func(err error) {
						var diagnostic *a2ui.ViewDiagnostic
						if errors.As(err, &diagnostic) {
							diagnostics[diagnostic.Code] = diagnostic.SurfaceID
						}
					},
					OpenURL:  func(target string) error { opened = append(opened, target); return nil },
					OnAction: func(message a2ui.RendererMessage) { actions = append(actions, *message.Action) },
				})
			}, 760, 640)
			tester.SetRegion(i18n.Region{Locale: "en-US", FirstWeekday: time.Sunday, HourCycle: 24})
			tester.UseScreenReader()
			for _, step := range tc.Steps {
				actions = nil
				surface := p.Surface(id)
				switch step.Op {
				case "process_messages":
					// The adapter fixtures still use unprefixed component
					// mixin aliases. Translate those to the 1.0 wire spelling.
					var stateMessages []map[string]any
					for _, message := range step.Messages {
						if response, ok := message["agentFunctionResponse"].(map[string]any); ok {
							response["functionCallId"] = callID
							raw, _ := json.Marshal(response)
							var result a2ui.FunctionResponse
							if err := json.Unmarshal(raw, &result); err != nil || !remoteOptions.Transport.Resolve(result) {
								t.Fatalf("missing pending call: %+v %v", result, err)
							}
							deadline := time.Now().Add(time.Second)
							for len(remoteErrors) == 0 && time.Now().Before(deadline) {
								tester.Frame()
								time.Sleep(time.Millisecond)
							}
							if len(remoteErrors) != 1 || remoteErrors[0].Error.Code != "EXPRESSION_ERROR" || remoteErrors[0].Error.FunctionCallID != callID {
								t.Fatalf("bad outbound error: %+v", remoteErrors)
							}
							continue
						}
						stateMessages = append(stateMessages, message)
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
					raw, err := json.Marshal(stateMessages)
					if err != nil {
						t.Fatal(err)
					}
					var messages []a2ui.AgentMessage
					if tc.StrictMode != nil && !*tc.StrictMode {
						for _, envelope := range step.Messages {
							data, err := json.Marshal(envelope)
							if err != nil {
								t.Fatal(err)
							}
							message, err := a2ui.DecodePermissive(data)
							if err != nil {
								t.Fatal(err)
							}
							messages = append(messages, message)
						}
					} else {
						messages, err = a2ui.DecodeBatch(raw)
					}
					if err != nil {
						t.Fatal(err)
					}
					if err := p.ApplyBatch(messages); err != nil {
						t.Fatal(err)
					}
					if tc.Name == "action_execution_event_vs_local_function_call" {
						if surface := p.Surface(id); surface != nil {
							surface.Data().EvaluateFunction = func(call a2ui.FunctionCall, _ string) (any, error) {
								if call.Name != "incrementCounter" {
									return nil, fmt.Errorf("unknown fixture function %s", call.Name)
								}
								path := a2ui.Text(call.Args["path"])
								before, _ := surface.Data().Get(path)
								return nil, surface.Data().Set(path, a2ui.Number(before)+a2ui.Number(call.Args["step"]))
							}
						}
					}
				case "programmatic_invoke":
					_, err := surface.Invoke(step.Target, "", time.Now(), false, func(target string) error { opened = append(opened, target); return nil })
					if err == nil || !strings.Contains(err.Error(), "openUrl requires user activation") || len(opened) != 1 || opened[0] != "https://example.com" {
						t.Fatalf("programmatic invocation: opened %v, error %v", opened, err)
					}
					raw, _ := json.Marshal(a2ui.RendererMessage{Version: a2ui.Version, Error: &a2ui.RendererError{Code: "EXPRESSION_ERROR", SurfaceID: &id, Message: err.Error()}})
					if _, err := a2ui.DecodeRenderer(raw); err != nil {
						t.Fatal(err)
					}
				case "unmount_surface":
					mounted = false
				case "user_input", "user_gesture":
					node, ok := surface.Component(step.Target)
					if !ok {
						t.Fatalf("unknown target %s", step.Target)
					}
					if step.Op == "user_input" && adapterControlInput(t, tester, surface, node, step.Input) {
						break
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
					if remoteOptions != nil {
						select {
						case event := <-remoteRequests:
							callID = event.CallAgent.ID
							if callID == "" || event.CallAgent.Function.Name != "lookupAccount" || event.CallAgent.Function.Args["accountId"] != "acc-123" {
								t.Fatalf("bad request %+v", event.CallAgent)
							}
						case <-time.After(time.Second):
							t.Fatal("missing remote function call")
						}
					}
					if node.Type == "TextField" {
						tester.Command("selectAll")
						tester.Type(step.Input.(string))
					}
				default:
					t.Fatalf("unhandled adapter operation %q", step.Op)
				}
				tester.Frame()
				if !mounted && len(tester.Texts()) != 0 {
					t.Fatal("unmounted surface remains visible")
				}
				surface = p.Surface(id)
				if step.SurfaceRemoved && (surface != nil || len(tester.Texts()) != 0) {
					t.Fatal("deleted surface is still visible")
				}
				if tc.Name == "fallback_states_unknown_type_and_cyclic_diagnostics" {
					for _, expected := range step.Errors {
						if diagnostics[expected.Code] != expected.SurfaceID {
							t.Errorf("missing %s diagnostic: %v", expected.Code, diagnostics)
						}
					}
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
	for prop, expected := range map[string]string{"url": want.URL, "name": want.Name} {
		if expected != "" {
			if got := surface.String(node.Props[prop], ""); got != expected {
				t.Errorf("%s.%s = %q, want %q", want.ID, prop, got, expected)
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

func adapterControlInput(t *testing.T, tester *ui.Tester, surface *a2ui.Surface, node a2ui.Component, value any) bool {
	t.Helper()
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	switch node.Type {
	case "ChoicePicker":
		var options []struct {
			Label json.RawMessage
			Value string
		}
		if err := json.Unmarshal(node.Props["options"], &options); err != nil {
			t.Fatal(err)
		}
		for _, selected := range value.([]any) {
			for _, option := range options {
				if option.Value == selected {
					click(surface.String(option.Label, ""))
				}
			}
		}
	case "Slider":
		for range 5 {
			if tester.Focused(adapterLabel(surface, node)) {
				break
			}
			tester.Key(0, ui.KeyTab)
		}
		if !tester.Focused(adapterLabel(surface, node)) {
			t.Fatalf("slider not focused: %v", tester.Texts())
		}
		tester.Key(0, ui.KeyHome)
		for range value.(int) {
			tester.Key(0, ui.KeyRight)
		}
	case "DateTimeInput":
		layout := "2006-01-02T15:04:05"
		date, err := time.Parse(layout, value.(string))
		if err != nil {
			t.Fatal(err)
		}
		previous, err := time.Parse(layout, surface.String(node.Props["value"], ""))
		if err != nil {
			t.Fatal(err)
		}
		click(previous.Format("1/2/2006"))
		click(date.Format("January 2, 2006"))
		click(previous.Format("15"))
		for _, digit := range date.Format("1504") {
			keys := []ui.Key{ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
			tester.Key(0, keys[digit-'0'])
		}
	default:
		return false
	}
	return true
}
