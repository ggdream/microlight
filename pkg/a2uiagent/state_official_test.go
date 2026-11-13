package a2uiagent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
	"gopkg.in/yaml.v3"
)

func TestOfficialRendererState(t *testing.T) {
	data, err := os.ReadFile("testdata/official/renderer_state.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string
		Catalogs []string
		Turns    []struct {
			Response string
			Payload  []map[string]any
			Surfaces map[string]struct {
				Data       any
				Components map[string]map[string]any
			} `yaml:"expect_surfaces"`
			Error map[string]any `yaml:"expect_error"`
		}
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			config := a2uiagent.Config{}
			state := new(a2ui.Processor)
			for _, path := range tc.Catalogs {
				config.Catalogs = append(config.Catalogs, catalogFor(t, filepath.Base(path)))
			}
			for i, turn := range tc.Turns {
				stream, err := config.NewStream(a2uiagent.Mixed, nil, state.Fork())
				if err != nil {
					t.Fatal(err)
				}
				response := turn.Response
				if len(turn.Payload) > 0 {
					raw, err := json.Marshal(turn.Payload)
					if err != nil {
						t.Fatal(err)
					}
					response = a2uiagent.Wrap(a2uiagent.DirectJSON, string(raw))
				}
				if _, err = stream.Push(response); err == nil {
					_, err = stream.Close()
				}
				if turn.Error != nil {
					if err == nil {
						t.Fatalf("turn %d: expected rejection", i+1)
					}
					continue
				}
				if err != nil {
					t.Fatalf("turn %d: %v", i+1, err)
				}
				state = stream.State
				for id, expected := range turn.Surfaces {
					surface := state.Surface(id)
					if surface == nil {
						t.Fatalf("turn %d: missing surface %s", i+1, id)
					}
					data, _ := surface.Data().Get("")
					if expected.Data != nil && !jsonEqual(data, expected.Data) {
						t.Errorf("turn %d: %s data: got %v want %v", i+1, id, data, expected.Data)
					}
					for node, want := range expected.Components {
						got, ok := surface.Component(node)
						if !ok {
							t.Fatalf("turn %d: missing %s/%s", i+1, id, node)
						}
						want["id"] = node
						if !jsonEqual(got, want) {
							t.Errorf("turn %d: %s/%s: got %+v want %v", i+1, id, node, got, want)
						}
					}
				}
				if len(turn.Surfaces) == 0 && strings.Contains(turn.Response, "deleteSurface") && state.Surface("s1") != nil {
					t.Errorf("turn %d: s1 survived deletion", i+1)
				}
			}
		})
	}
}
