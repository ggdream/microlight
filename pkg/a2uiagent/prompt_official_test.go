package a2uiagent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
	"gopkg.in/yaml.v3"
)

func TestOfficialPromptGeneration(t *testing.T) {
	for _, suite := range []string{"express_prompt_generator", "direct_json_prompt_generator"} {
		raw, err := os.ReadFile("testdata/official/" + suite + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Name string
			Args struct {
				Format   a2uiagent.Format
				Catalogs []struct {
					Catalog      any
					Transformers []map[string][]string
				}
				Examples []string
				Allowed  []string `yaml:"allowed_messages"`
			}
			Contains      []string       `yaml:"expect_contains"`
			Absent        []string       `yaml:"expect_absent"`
			ExpectFile    string         `yaml:"expect_file"`
			Deterministic bool           `yaml:"expect_deterministic"`
			ExpectError   map[string]any `yaml:"expect_error"`
		}
		if err := yaml.Unmarshal(raw, &cases); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			t.Run(suite+"/"+tc.Name, func(t *testing.T) {
				var catalogs []*a2ui.Catalog
				for _, fixture := range tc.Args.Catalogs {
					catalog := catalogFor(t, fixture.Catalog)
					catalogs = append(catalogs, transformFixtureCatalog(t, catalog, fixture.Transformers))
				}
				options := a2uiagent.PromptOptions{AllowedMessages: tc.Args.Allowed}
				for _, path := range tc.Args.Examples {
					raw, err := os.ReadFile("testdata/official/" + filepath.Base(path))
					if err != nil {
						t.Fatal(err)
					}
					messages, err := a2ui.DecodeBatch(raw)
					if err != nil {
						t.Fatal(err)
					}
					options.Examples = append(options.Examples, messages)
				}
				build := func() (string, error) {
					return a2uiagent.BuildPrompt(tc.Args.Format, catalogs, a2ui.RendererDataModel{}, options)
				}
				got, err := build()
				if tc.ExpectError != nil {
					if err == nil {
						t.Fatal("expected missing catalog error")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				for _, fragment := range tc.Contains {
					if tc.Args.Format == a2uiagent.DirectJSON && strings.Contains(fragment, "\n</a2ui-json>") {
						assertDirectPrompt(t, got, catalogs, options)
						continue
					}
					if !strings.Contains(got, fragment) {
						t.Errorf("missing %q in %s", fragment, got)
					}
				}
				for _, fragment := range tc.Absent {
					if strings.Contains(got, fragment) {
						t.Errorf("unexpected %q in %s", fragment, got)
					}
				}
				if tc.ExpectFile != "" {
					assertDirectPrompt(t, got, catalogs, options)
				}
				if tc.Deterministic {
					repeated, err := build()
					if err != nil || repeated != got {
						t.Fatal("prompt generation is not deterministic")
					}
				}
			})
		}
	}
}

func assertDirectPrompt(t *testing.T, got string, catalogs []*a2ui.Catalog, options a2uiagent.PromptOptions) {
	t.Helper()
	for _, token := range []string{"<a2ui-json>", "</a2ui-json>"} {
		if !strings.Contains(got, token) {
			t.Errorf("missing sentinel %s", token)
		}
	}
	for _, catalog := range catalogs {
		if !strings.Contains(got, catalog.ID) {
			t.Errorf("missing catalog %s", catalog.ID)
		}
		for name := range catalog.Components {
			if !strings.Contains(got, "`"+name+"`") && !strings.Contains(got, "\""+name+"\"") {
				t.Errorf("missing component %s", name)
			}
		}
	}
	if options.AllowedMessages != nil && strings.Contains(got, "- deleteSurface:") {
		t.Error("pruned payload still appears")
	}
	_, section, _ := strings.Cut(got, "\n\nExamples\n\n")
	parts := a2uiagent.UnwrapResponse(a2uiagent.DirectJSON, section)
	if len(parts) != len(options.Examples) {
		t.Fatalf("got %d examples, want %d", len(parts), len(options.Examples))
	}
	for i, part := range parts {
		messages, err := a2ui.DecodeBatch([]byte(*part.Payload))
		if err != nil || !jsonEqual(messages, options.Examples[i]) {
			t.Fatalf("example %d did not round trip: %v", i+1, err)
		}
	}
}
