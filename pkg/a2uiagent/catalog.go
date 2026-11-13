package a2uiagent

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/ggdream/microlight/pkg/a2ui"
)

// CatalogTransformer builds a catalog for a particular agent registration.
type CatalogTransformer func(*a2ui.Catalog) (*a2ui.Catalog, error)

func KeepComponents(names ...string) CatalogTransformer {
	return pruneCatalog("components", names)
}

func KeepFunctions(names ...string) CatalogTransformer {
	return pruneCatalog("functions", names)
}

func pruneCatalog(field string, names []string) CatalogTransformer {
	names = slices.Clone(names)
	return func(catalog *a2ui.Catalog) (*a2ui.Catalog, error) {
		var document map[string]json.RawMessage
		if err := json.Unmarshal(catalog.Document(), &document); err != nil {
			return nil, err
		}
		declarations := catalog.Components
		if field == "functions" {
			declarations = catalog.Functions
		}
		kept := maps.Clone(declarations)
		maps.DeleteFunc(kept, func(name string, _ json.RawMessage) bool {
			return !slices.Contains(names, name)
		})
		if kept == nil {
			kept = map[string]json.RawMessage{}
		}
		document[field], _ = json.Marshal(kept)
		// Remove alternatives that point to declarations the agent no longer holds.
		var defs map[string]json.RawMessage
		if json.Unmarshal(document["$defs"], &defs) == nil {
			key := "anyComponent"
			if field == "functions" {
				key = "anyFunction"
			}
			var union map[string]json.RawMessage
			if json.Unmarshal(defs[key], &union) == nil {
				var branches []map[string]json.RawMessage
				if json.Unmarshal(union["oneOf"], &branches) == nil {
					prefix := "#/" + field + "/"
					branches = slices.DeleteFunc(branches, func(branch map[string]json.RawMessage) bool {
						var ref string
						_ = json.Unmarshal(branch["$ref"], &ref)
						return len(ref) > len(prefix) && ref[:len(prefix)] == prefix && kept[ref[len(prefix):]] == nil
					})
					if len(branches) == 0 {
						defs[key] = json.RawMessage(`{"not":{}}`)
					} else {
						union["oneOf"], _ = json.Marshal(branches)
						defs[key], _ = json.Marshal(union)
					}
					document["$defs"], _ = json.Marshal(defs)
				}
			}
		}
		raw, err := json.Marshal(document)
		if err != nil {
			return nil, err
		}
		return a2ui.ParseCatalog(raw)
	}
}

// TransformCatalog applies the supplied transformers in registration order.
func TransformCatalog(catalog *a2ui.Catalog, transforms ...CatalogTransformer) (*a2ui.Catalog, error) {
	if catalog == nil {
		return nil, fmt.Errorf("missing catalog")
	}
	for _, transform := range transforms {
		var err error
		catalog, err = transform(catalog)
		if err != nil {
			return nil, err
		}
	}
	return catalog, nil
}
