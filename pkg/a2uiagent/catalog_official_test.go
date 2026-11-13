package a2uiagent_test

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
	"gopkg.in/yaml.v3"
)

func TestOfficialCatalogResolution(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/catalog_resolution.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string
		Args struct {
			Catalogs []struct {
				Catalog      any
				Transformers []map[string][]string
			}
			AcceptInline bool           `yaml:"accepts_inline_catalogs"`
			Renderer     map[string]any `yaml:"renderer_capabilities"`
		}
		Expect struct {
			IDs      []string `yaml:"active_catalog_ids"`
			Catalogs []struct {
				ID         string `yaml:"catalog_id"`
				Components []string
				Functions  []string
			}
		}
		ExpectError any `yaml:"expect_error"`
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			config := a2uiagent.Config{Catalogs: make([]*a2ui.Catalog, 0), AcceptInline: tc.Args.AcceptInline}
			for _, registration := range tc.Args.Catalogs {
				catalog := catalogFor(t, registration.Catalog)
				catalog = transformFixtureCatalog(t, catalog, registration.Transformers)
				config.Catalogs = append(config.Catalogs, catalog)
			}
			encoded, err := json.Marshal(tc.Args.Renderer)
			if err != nil {
				t.Fatal(err)
			}
			var renderer a2ui.RendererCapabilities
			err = json.Unmarshal(encoded, &renderer)
			var actual []*a2ui.Catalog
			if err == nil {
				actual, err = config.Negotiate(renderer)
			}
			if (err != nil) != (tc.ExpectError != nil) {
				t.Fatalf("negotiation error %v, want %v", err, tc.ExpectError)
			}
			if err != nil {
				return
			}
			ids := make([]string, 0, len(actual))
			for _, catalog := range actual {
				ids = append(ids, catalog.ID)
			}
			if !slices.Equal(ids, tc.Expect.IDs) {
				t.Fatalf("catalog IDs %v, want %v", ids, tc.Expect.IDs)
			}
			for _, expected := range tc.Expect.Catalogs {
				index := slices.Index(ids, expected.ID)
				if index < 0 {
					t.Fatalf("missing catalog %s", expected.ID)
				}
				catalog := actual[index]
				for _, pair := range []struct {
					want []string
					got  map[string]json.RawMessage
				}{{expected.Components, catalog.Components}, {expected.Functions, catalog.Functions}} {
					if pair.want == nil {
						continue
					}
					slices.Sort(pair.want)
					if got := slices.Sorted(maps.Keys(pair.got)); !slices.Equal(got, pair.want) {
						t.Errorf("catalog %s declarations %v, want %v", expected.ID, got, pair.want)
					}
				}
			}
		})
	}
}

func transformFixtureCatalog(t *testing.T, catalog *a2ui.Catalog, directives []map[string][]string) *a2ui.Catalog {
	t.Helper()
	var transforms []a2uiagent.CatalogTransformer
	for _, directive := range directives {
		for name, keep := range directive {
			switch name {
			case "component_pruning":
				transforms = append(transforms, a2uiagent.KeepComponents(keep...))
			case "function_pruning":
				transforms = append(transforms, a2uiagent.KeepFunctions(keep...))
			default:
				t.Fatalf("unknown catalog transformer %q", name)
			}
		}
	}
	result, err := a2uiagent.TransformCatalog(catalog, transforms...)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOfficialCatalogTransformers(t *testing.T) {
	raw, err := os.ReadFile("testdata/official/catalog_transformer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string
		Args struct {
			Catalog      any
			Transformers []map[string][]string
		}
		Expect struct {
			ID         string `yaml:"catalog_id"`
			Version    string `yaml:"protocol_version"`
			Components []string
			Functions  []string
		}
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Args.Catalog == "test_data/catalogs/catalog_without_id.json" {
				t.Skip("anonymous pre-1.0 catalog")
			}
			catalog := transformFixtureCatalog(t, catalogFor(t, tc.Args.Catalog), tc.Args.Transformers)
			if tc.Expect.ID != "" && tc.Expect.ID != catalog.ID {
				t.Errorf("catalog ID %q, want %q", catalog.ID, tc.Expect.ID)
			}
			if tc.Expect.Version != "" && tc.Expect.Version != catalog.Version {
				t.Errorf("catalog version %q, want %q", catalog.Version, tc.Expect.Version)
			}
			for _, pair := range []struct {
				want []string
				got  map[string]json.RawMessage
			}{{tc.Expect.Components, catalog.Components}, {tc.Expect.Functions, catalog.Functions}} {
				if pair.want != nil {
					slices.Sort(pair.want)
					if got := slices.Sorted(maps.Keys(pair.got)); !slices.Equal(got, pair.want) {
						t.Errorf("declarations %v, want %v", got, pair.want)
					}
				}
			}
		})
	}
}
