package a2ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed schema/*.json
var schemaFiles embed.FS

const schemaBase = "https://a2ui.org/specification/v1_0/"

type Catalog struct {
	ID           string                     `json:"catalogId"`
	Version      string                     `json:"protocolVersion"`
	Title        string                     `json:"title"`
	Instructions string                     `json:"instructions"`
	Components   map[string]json.RawMessage `json:"components"`
	Functions    map[string]json.RawMessage `json:"functions"`
	Definitions  map[string]json.RawMessage `json:"$defs"`
	raw          json.RawMessage
	validators   sync.Map
}

func ParseCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.ID == "" || c.Version != Version || (c.Components == nil && c.Functions == nil) {
		return nil, fmt.Errorf("expected a named A2UI %s catalog with components or functions", Version)
	}
	c.raw = append(json.RawMessage(nil), data...)
	return &c, nil
}

func BasicCatalogDocument() json.RawMessage {
	raw, _ := schemaFiles.ReadFile("schema/basic.json")
	return raw
}

var basicCatalog = sync.OnceValues(func() (*Catalog, error) {
	return ParseCatalog(BasicCatalogDocument())
})

func (c *Catalog) Document() json.RawMessage { return append(json.RawMessage(nil), c.raw...) }

var identifierPattern = regexp.MustCompile(`^[_\pL\p{Nl}\x{1885}\x{1886}\x{2118}\x{212e}\x{309b}\x{309c}][_\pL\p{Nl}\p{Mn}\p{Mc}\p{Nd}\p{Pc}\x{1885}\x{1886}\x{2118}\x{212e}\x{309b}\x{309c}\x{b7}\x{387}\x{1369}-\x{1371}\x{19da}]*$`)

func (c *Catalog) ValidateComponent(component Component) error {
	if !identifierPattern.MatchString(component.ID) || !identifierPattern.MatchString(component.Type) {
		return fmt.Errorf("invalid component identifier %q (%s)", component.ID, component.Type)
	}
	schema, ok := c.Components[component.Type]
	if !ok {
		return fmt.Errorf("catalog %q has no component %q", c.ID, component.Type)
	}
	validator, err := c.validator("component:"+component.Type, schema, true)
	if err != nil {
		return err
	}
	data, err := json.Marshal(component)
	if err != nil {
		return err
	}
	return validateJSON(validator, data)
}

func (c *Catalog) ValidateFunction(call FunctionCall) error {
	schema, ok := c.Functions[call.Name]
	if !ok {
		return fmt.Errorf("catalog %q has no function %q", c.ID, call.Name)
	}
	validator, err := c.validator("function:"+call.Name, schema, false)
	if err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Name    string          `json:"@call"`
		Catalog string          `json:"catalogId,omitempty"`
		Args    *map[string]any `json:"args,omitempty"`
	}{Name: call.Name, Catalog: call.Catalog, Args: argumentPointer(call.Args)})
	if err != nil {
		return err
	}
	return validateJSON(validator, data)
}

func (c *Catalog) validator(key string, definition json.RawMessage, component bool) (*jsonschema.Resolved, error) {
	if previous, ok := c.validators.Load(key); ok {
		return previous.(*jsonschema.Resolved), nil
	}
	definitions := make(map[string]json.RawMessage, len(c.Definitions)+len(c.Components))
	for name, schema := range c.Definitions {
		definitions[name] = schema
	}
	for name, schema := range c.Components {
		definitions["component_"+name] = schema
	}
	for name, schema := range c.Functions {
		definitions["function_"+name] = schema
	}
	doc := map[string]any{"$defs": definitions}
	if component {
		var schema map[string]json.RawMessage
		if err := json.Unmarshal(definition, &schema); err != nil {
			return nil, err
		}
		// Closing the composed schema lets a catalog with closed top-level
		// properties receive the component envelope fields as well.
		var props map[string]json.RawMessage
		if json.Unmarshal(schema["properties"], &props) == nil {
			for _, name := range []string{"catalogId", "metadata"} {
				if props[name] != nil && !customProperty(definition, name) {
					delete(props, name)
				}
			}
			schema["properties"], _ = json.Marshal(props)
		}
		if string(schema["additionalProperties"]) == "false" {
			delete(schema, "additionalProperties")
		}
		definition, _ = json.Marshal(schema)
		doc["allOf"] = []any{map[string]string{"$ref": schemaBase + "common_types.json#/$defs/ComponentCommon"}, definition}
		doc["unevaluatedProperties"] = false
	} else {
		doc["allOf"] = []any{definition}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	validator, err := compileSchema(data, schemaBase+"catalog-entry.json")
	if err != nil {
		return nil, err
	}
	actual, _ := c.validators.LoadOrStore(key, validator)
	return actual.(*jsonschema.Resolved), nil
}

var envelopeSchemas sync.Map

// ValidateProtocol validates an A2UI 1.0 wire document against its official schema.
// Catalog-specific validation is performed separately using the negotiated catalog.
func ValidateProtocol(schemaName string, data []byte) error {
	if schemaName != path.Base(schemaName) || !strings.HasSuffix(schemaName, ".json") {
		return fmt.Errorf("invalid A2UI schema name %q", schemaName)
	}
	cached, ok := envelopeSchemas.Load(schemaName)
	if !ok {
		source, err := schemaFiles.ReadFile("schema/" + schemaName)
		if err != nil {
			return err
		}
		if schemaName == "agent_to_renderer.json" {
			// The envelope's closed component schema needs the selected
			// catalog. Validate that part separately, for each component.
			var doc struct {
				Defs map[string]json.RawMessage `json:"$defs"`
			}
			var root map[string]json.RawMessage
			if err = json.Unmarshal(source, &root); err != nil {
				return err
			}
			if err = json.Unmarshal(source, &doc); err != nil {
				return err
			}
			doc.Defs["Component"] = json.RawMessage(`{"$ref":"common_types.json#/$defs/ComponentCommon"}`)
			root["$defs"], _ = json.Marshal(doc.Defs)
			source, _ = json.Marshal(root)
		}
		compiled, err := compileSchema(source, schemaBase+schemaName)
		if err != nil {
			return err
		}
		cached, _ = envelopeSchemas.LoadOrStore(schemaName, compiled)
	}
	return validateJSON(cached.(*jsonschema.Resolved), data)
}

func compileSchema(data []byte, base string) (*jsonschema.Resolved, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(portablePatterns(data), &schema); err != nil {
		return nil, err
	}
	return schema.Resolve(&jsonschema.ResolveOptions{
		BaseURI: base,
		Loader: func(uri *url.URL) (*jsonschema.Schema, error) {
			var doc jsonschema.Schema
			switch uri.String() {
			case schemaBase + "catalog.json":
				return &jsonschema.Schema{Defs: map[string]*jsonschema.Schema{"anyFunction": {Properties: map[string]*jsonschema.Schema{"args": {Type: "object"}}, Not: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"@call": {Const: new(any("@index"))}}, Required: []string{"@call"}}}}}, nil
			case schemaBase + "common_types.json", schemaBase + "catalog_definition.json":
				data, err := schemaFiles.ReadFile("schema/" + path.Base(uri.Path))
				if err != nil {
					return nil, err
				}
				if err := json.Unmarshal(portablePatterns(data), &doc); err != nil {
					return nil, err
				}
				return &doc, nil
			default:
				return nil, fmt.Errorf("unregistered schema reference %q", uri)
			}
		},
	})
}

func validateJSON(schema *jsonschema.Resolved, data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return schema.Validate(value)
}

// The official schemas use ECMAScript XID character classes. Go's regexp
// accepts Unicode general categories and Other_ID properties instead.
func portablePatterns(data []byte) []byte {
	return []byte(strings.NewReplacer(
		`"#/components/`, `"#/$defs/component_`,
		`"#/functions/`, `"#/$defs/function_`,
		`\\p{XID_Start}`, `\\pL\\p{Nl}\\x{1885}\\x{1886}\\x{2118}\\x{212e}\\x{309b}\\x{309c}`,
		`\\p{XID_Continue}`, `\\pL\\p{Nl}\\p{Mn}\\p{Mc}\\p{Nd}\\p{Pc}\\x{1885}\\x{1886}\\x{2118}\\x{212e}\\x{309b}\\x{309c}\\x{b7}\\x{387}\\x{1369}-\\x{1371}\\x{19da}`,
	).Replace(string(data)))
}

func (c *Catalog) RendererMayCall(name string) bool {
	var declaration struct {
		Callers string `json:"allowedCallers"`
	}
	_ = json.Unmarshal(c.Functions[name], &declaration)
	return declaration.Callers != "agentOnly"
}

func (c *Catalog) AgentMayCall(name string) bool {
	var declaration struct {
		Callers    string `json:"allowedCallers"`
		Activation bool   `json:"requiresUserActivation"`
	}
	_ = json.Unmarshal(c.Functions[name], &declaration)
	return !declaration.Activation && (declaration.Callers == "agentOnly" || declaration.Callers == "rendererOrAgent")
}

// FunctionArguments validates the call before applying optional argument defaults.
func (c *Catalog) FunctionArguments(call FunctionCall) (map[string]any, error) {
	if err := c.ValidateFunction(call); err != nil {
		return nil, err
	}
	var definition struct{ Properties map[string]json.RawMessage }
	if err := json.Unmarshal(c.Functions[call.Name], &definition); err != nil {
		return nil, err
	}
	args, _ := cloneJSON(call.Args).(map[string]any)
	if args == nil {
		args = make(map[string]any)
	}
	if err := applyFunctionDefaults(args, definition.Properties["args"], c.Definitions, 0); err != nil {
		return nil, err
	}
	call.Args = args
	if err := c.ValidateFunction(call); err != nil {
		return nil, err
	}
	return args, nil
}

func argumentPointer(args map[string]any) *map[string]any {
	if args == nil {
		return nil
	}
	return &args
}

func (c *Catalog) requiresActivation(name string) bool {
	var declaration struct {
		Requires bool `json:"requiresUserActivation"`
	}
	_ = json.Unmarshal(c.Functions[name], &declaration)
	return declaration.Requires
}
