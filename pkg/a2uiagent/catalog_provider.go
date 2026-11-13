package a2uiagent

import (
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/ggdream/microlight/pkg/a2ui"
)

// LoadCatalog loads a catalog from a host-owned filesystem. Identity can be
// supplied externally when the document omits it.
func LoadCatalog(source fs.FS, path, id, version string) (*a2ui.Catalog, error) {
	raw, err := fs.ReadFile(source, path)
	if err != nil {
		return nil, err
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, fmt.Errorf("invalid A2UI catalog document")
	}
	for _, field := range []struct{ name, expected string }{{"catalogId", id}, {"protocolVersion", version}} {
		if field.expected == "" {
			continue
		}
		if value, exists := document[field.name]; exists {
			var actual string
			if json.Unmarshal(value, &actual) != nil || actual != field.expected {
				return nil, fmt.Errorf("catalog %s conflicts with provider", field.name)
			}
		} else {
			document[field.name], _ = json.Marshal(field.expected)
		}
	}
	raw, err = json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return a2ui.ParseCatalog(raw)
}
