package a2uimygo_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
	"gopkg.in/yaml.v3"
)

type accessAssertions struct {
	Tree map[string]struct {
		Label       any
		Description string
		Role        string
		Live        string
		Hidden      any
		Checked     *bool
	} `yaml:"accessibilityTree"`
}

type accessibilityStep struct {
	Messages   []map[string]any
	Assertions accessAssertions
}

func TestOfficialMyGoAccessibility(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/accessibility.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name              string
		accessibilityStep `yaml:",inline"`
		Steps             []accessibilityStep
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	roles := map[string]ui.Role{
		"button": ui.RoleButton, "textbox": ui.RoleTextField,
		"checkbox": ui.RoleCheckBox, "radiogroup": ui.RoleRadioGroup,
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var p a2ui.Processor
			var id string
			tester := ui.NewTester(func(c *ui.Context) {
				a2uimygo.View(c, p.Surface(id), a2uimygo.Options{})
			}, 640, 480)
			tester.UseScreenReader()
			steps := tc.Steps
			if len(steps) == 0 {
				steps = []accessibilityStep{tc.accessibilityStep}
			}
			for _, step := range steps {
				raw, err := json.Marshal(step.Messages)
				if err != nil {
					t.Fatal(err)
				}
				messages, err := a2ui.DecodeBatch(raw)
				if err != nil {
					t.Fatal(err)
				}
				for _, message := range messages {
					if message.Create != nil {
						id = message.Create.ID
					}
				}
				if err := p.ApplyBatch(messages); err != nil {
					t.Fatal(err)
				}
				tester.Frame()
				surface := p.Surface(id)
				for componentID, want := range step.Assertions.Tree {
					node, ok := surface.Component(componentID)
					if !ok {
						t.Fatalf("missing component %s", componentID)
					}
					label, _ := want.Label.(string)
					if label == "" {
						label = adapterLabel(surface, node)
					}
					if want.Hidden == true {
						if componentID == a2ui.Root {
							if got := tester.AccessAll(); len(got) != 0 {
								t.Errorf("hidden root exposes %+v", got)
							}
						} else if _, exposed := tester.Access(label); exposed {
							t.Errorf("hidden component %s exposes %q", componentID, label)
						}
						continue
					}
					role := roles[want.Role]
					switch want.Live {
					case "polite":
						role = ui.RoleStatus
					case "assertive":
						role = ui.RoleAlert
					}
					info, exposed := tester.Access(label)
					if role != ui.RoleAuto {
						info, exposed = tester.AccessByRole(role, label)
					}
					if !exposed || (want.Description != "" && want.Description != info.Description) || (want.Checked != nil && *want.Checked != info.Checked) {
						t.Errorf("component %s (%q) accessibility mismatch: %+v, exposed=%v", componentID, label, info, exposed)
					}
				}
			}
		})
	}
}
