package a2uiagent_test

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
	"gopkg.in/yaml.v3"
)

type functionFixtureCatalog struct {
	Catalog   any
	Functions map[string]struct {
		Name        string
		Description string
		FromAPI     bool   `yaml:"from_api"`
		ReturnType  string `yaml:"returnType"`
		Activation  bool   `yaml:"requiresUserActivation"`
		Args        map[string]any
		Behavior    map[string]any
	} `yaml:"agent_functions"`
}

func TestOfficialAgentFunctionHandling(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/agent_functions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Action string
		Args         struct {
			Catalogs []functionFixtureCatalog
			Message  map[string]any
		}
		Expect      map[string]any
		ExpectError map[string]any `yaml:"expect_error"`
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		if tc.Action != "handle_agent_function" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			handlers := make(map[string]map[string]a2uiagent.AgentFunction)
			var catalogs []*a2ui.Catalog
			for _, fixture := range tc.Args.Catalogs {
				catalog, registered := registerFixtureFunctions(t, fixture)
				catalogs = append(catalogs, catalog)
				handlers[catalog.ID] = registered
			}
			data, err := json.Marshal(tc.Args.Message)
			if err != nil {
				t.Fatal(err)
			}
			reply, err := a2uiagent.HandleFunctionMessage(t.Context(), catalogs, handlers, data)
			if tc.ExpectError != nil {
				if err == nil {
					t.Fatal("expected invalid envelope")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wire, _ := json.Marshal(reply.AgentResponse)
			var result map[string]any
			_ = json.Unmarshal(wire, &result)
			for key, value := range tc.Expect {
				if key == "error" {
					expected := value.(map[string]any)
					actual, _ := result[key].(map[string]any)
					for field, want := range expected {
						if field == "message" {
							got, _ := actual[field].(string)
							if strings.Contains(got, want.(string)) {
								continue
							}
						}
						if !reflect.DeepEqual(actual[field], want) {
							t.Fatalf("%s: got %s want %v", field, wire, value)
						}
					}
				} else if !jsonEqual(result[key], value) {
					t.Fatalf("%s: got %s want %v", key, wire, value)
				}
			}
		})
	}
}

func registerFixtureFunctions(t *testing.T, fixture functionFixtureCatalog) (*a2ui.Catalog, map[string]a2uiagent.AgentFunction) {
	t.Helper()
	catalog, handlers, err := a2uiagent.RegisterFunctions(catalogFor(t, fixture.Catalog), fixtureRegistrations(fixture))
	if err != nil {
		t.Fatal(err)
	}
	return catalog, handlers
}

func fixtureRegistrations(fixture functionFixtureCatalog) []a2uiagent.FunctionRegistration {
	var registrations []a2uiagent.FunctionRegistration
	for name, declaration := range fixture.Functions {
		if declaration.Name != "" {
			name = declaration.Name
		}
		var raw json.RawMessage
		if declaration.Args != nil {
			raw, _ = json.Marshal(declaration.Args)
		}
		handler := func(ctx context.Context, surface string, args map[string]any) (any, error) {
			switch {
			case declaration.Behavior["returns_context"] == true:
				call, _ := a2uiagent.FunctionCallFromContext(ctx)
				return map[string]string{"catalogId": call.Function.Catalog, "functionCallId": call.ID, "surfaceId": surface}, nil
			case declaration.Behavior["returns_args"] == true:
				return args, nil
			}
			if detail, ok := declaration.Behavior["raises"].(map[string]any); ok {
				code, _ := detail["code"].(string)
				message, _ := detail["message"].(string)
				return nil, &a2ui.FunctionError{Code: code, Message: message}
			}
			return declaration.Behavior["returns"], nil
		}
		registrations = append(registrations, a2uiagent.FunctionRegistration{Name: name, FromCatalog: declaration.FromAPI, ReturnType: declaration.ReturnType, Description: declaration.Description, RequiresUserActivation: declaration.Activation, Arguments: raw, Handler: handler})
	}
	return registrations
}

func TestOfficialRendererMessageRouting(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/agent_functions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Action string
		Args         struct {
			Catalogs  []functionFixtureCatalog
			Catalog   *functionFixtureCatalog
			Callbacks []any
			Message   map[string]any
		}
		Expect struct {
			Handled  bool
			Callback map[string]any
			Replies  []map[string]any
		}
		ExpectError map[string]any `yaml:"expect_error"`
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		if tc.Action != "process_renderer_message" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			handlers := make(map[string]map[string]a2uiagent.AgentFunction)
			var catalogs []*a2ui.Catalog
			fixtures := tc.Args.Catalogs
			if tc.Args.Catalog != nil {
				fixtures = append(fixtures, *tc.Args.Catalog)
			}
			for _, fixture := range fixtures {
				catalog, registered := registerFixtureFunctions(t, fixture)
				catalogs = append(catalogs, catalog)
				handlers[catalog.ID] = registered
			}
			var actual map[string]any
			var callbacks a2uiagent.RendererCallbacks
			for _, callback := range tc.Args.Callbacks {
				kind, _ := callback.(string)
				accept := true
				if details, ok := callback.(map[string]any); ok {
					kind, _ = details["kind"].(string)
					accept = details["declines"] != true
				}
				call := func(message a2ui.RendererMessage) bool {
					actual = map[string]any{"kind": kind, "message": message}
					return accept
				}
				switch kind {
				case "action":
					callbacks.Action = func(event a2ui.ActionEvent) bool {
						return call(a2ui.RendererMessage{Version: a2ui.Version, Action: &event})
					}
				case "error":
					callbacks.Error = func(event a2ui.RendererError) bool {
						return call(a2ui.RendererMessage{Version: a2ui.Version, Error: &event})
					}
				case "rendererFunctionResponse":
					callbacks.RendererResponse = func(event a2ui.FunctionResponse) bool {
						return call(a2ui.RendererMessage{Version: a2ui.Version, RendererResponse: &event})
					}
				}
			}
			raw, _ := json.Marshal(tc.Args.Message)
			handled, replies, err := a2uiagent.ProcessRendererMessage(t.Context(), raw, catalogs, handlers, callbacks)
			if tc.ExpectError != nil {
				if err == nil {
					t.Fatal("expected invalid envelope or catalog")
				}
				return
			}
			if err != nil || handled != tc.Expect.Handled || !jsonEqual(actual, tc.Expect.Callback) || len(replies) != len(tc.Expect.Replies) {
				t.Fatalf("routing: handled=%v, callback=%+v, replies=%+v, err=%v", handled, actual, replies, err)
			}
			for i, reply := range replies {
				if expected, _ := tc.Expect.Replies[i]["agentFunctionResponse"].(map[string]any); expected["error"] != nil {
					detail := expected["error"].(map[string]any)
					if reply.AgentResponse.Error == nil || reply.AgentResponse.Error.Code != detail["code"] {
						t.Fatalf("unexpected error %+v", reply.AgentResponse.Error)
					}
					detail["message"] = reply.AgentResponse.Error.Message
				}
				if !jsonEqual(reply, tc.Expect.Replies[i]) {
					t.Fatalf("unexpected reply %+v", reply)
				}
			}
		})
	}
}

func TestOfficialAgentFunctionRegistration(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/agent_functions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Action string
		Args         struct {
			Catalog  functionFixtureCatalog
			Catalogs []functionFixtureCatalog
			Messages []map[string]any
		}
		Expect struct {
			Functions         []string
			AgentFunctions    []string `yaml:"agent_functions"`
			DocumentFunctions []string `yaml:"document_functions"`
		}
		ExpectError map[string]any `yaml:"expect_error"`
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		switch tc.Action {
		case "declare_agent_functions":
			t.Run(tc.Name, func(t *testing.T) {
				fixture := tc.Args.Catalog
				catalog, handlers, err := a2uiagent.RegisterFunctions(catalogFor(t, fixture.Catalog), fixtureRegistrations(fixture))
				if tc.ExpectError != nil {
					if err == nil {
						t.Fatal("expected invalid declaration")
					}
					if want, ok := tc.ExpectError["message"].(string); ok && !strings.Contains(err.Error(), want) {
						t.Fatalf("missing %q in %v", want, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var doc struct{ Functions map[string]json.RawMessage }
				if err := json.Unmarshal(catalog.Document(), &doc); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(slices.Sorted(maps.Keys(catalog.Functions)), sortedCopy(tc.Expect.Functions)) ||
					!slices.Equal(slices.Sorted(maps.Keys(handlers)), sortedCopy(tc.Expect.AgentFunctions)) ||
					!slices.Equal(slices.Sorted(maps.Keys(doc.Functions)), sortedCopy(tc.Expect.DocumentFunctions)) {
					t.Fatal("registered functions and catalog declarations differ")
				}
			})
		case "validate_payload":
			t.Run(tc.Name, func(t *testing.T) {
				var catalogs []*a2ui.Catalog
				for _, fixture := range tc.Args.Catalogs {
					catalog, _ := registerFixtureFunctions(t, fixture)
					catalogs = append(catalogs, catalog)
				}
				raw, _ := json.Marshal(tc.Args.Messages)
				_, err := a2uiagent.CompileDirectJSON(string(raw), catalogs)
				if (err == nil) == (tc.ExpectError != nil) {
					t.Fatalf("unexpected validation result: %v", err)
				}
			})
		}
	}
}
