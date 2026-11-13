package a2uiagent_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ggdream/microlight/pkg/a2uiagent"
	"gopkg.in/yaml.v3"
)

func TestOfficialCatalogProvider(t *testing.T) {
	source := os.DirFS("testdata/official")
	raw, err := os.ReadFile("testdata/official/catalog_provider.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type catalogFields struct {
		ID      string `yaml:"catalog_id"`
		Version string `yaml:"protocol_version"`
	}
	var cases []struct {
		Name string
		Args struct {
			catalogFields `yaml:",inline"`
			Path          string
		}
		Expect struct {
			catalogFields `yaml:",inline"`
			Components    []string
			Functions     []string
		}
		Error map[string]any `yaml:"expect_error"`
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			catalog, err := a2uiagent.LoadCatalog(source, filepath.Base(tc.Args.Path), tc.Args.ID, tc.Args.Version)
			if tc.Error != nil {
				if err == nil {
					t.Fatal("expected catalog rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if catalog.ID != tc.Expect.ID || tc.Expect.Version != "" && catalog.Version != tc.Expect.Version {
				t.Fatalf("incorrect catalog identity: %s %s", catalog.ID, catalog.Version)
			}
			if want := tc.Expect.Components; want != nil && !slices.Equal(slices.Sorted(maps.Keys(catalog.Components)), sortedCopy(want)) {
				t.Fatal("incorrect components")
			}
			if want := tc.Expect.Functions; want != nil && !slices.Equal(slices.Sorted(maps.Keys(catalog.Functions)), sortedCopy(want)) {
				t.Fatal("incorrect functions")
			}
		})
	}
}

func sortedCopy(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return out
}
