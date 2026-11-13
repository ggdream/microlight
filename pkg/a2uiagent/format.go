package a2uiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"
)

type Format string

const (
	DirectJSON Format = "direct_json"
	Express    Format = "express"
	// Mixed accepts both tagged inference formats in the same response.
	Mixed Format = "mixed"
)

// Wrap formats model-facing content using the format's sentinel tags.
func Wrap(format Format, payload string) string {
	if format == Express {
		return "<a2ui>\n" + payload + "\n</a2ui>"
	}
	return "<a2ui-json>\n" + payload + "\n</a2ui-json>"
}

func PromptFor(format Format, catalogs []*a2ui.Catalog, state a2ui.RendererDataModel) (string, error) {
	prompt, err := Prompt(catalogs, state)
	if err != nil || format == DirectJSON || format == "" {
		return prompt, err
	}
	if format == Mixed {
		return `Prefer Express for interactive replies. Use bare tags on their own lines and never put the UI inside a Markdown code fence. Example:
<a2ui>
surface("welcome")
root = Text("Hello")
</a2ui>
Express: start with surface("id"), declare root = Component(...), and declare children as variables referenced by identifier. Use named properties, e.g. Button(child=label, action=Event("submit", {value: $/value})). Bind with $/path and set data with $/path = value. Start with surface("id", update=true) to update an existing surface.
Use the following JSON format as an alternative when exact wire messages are needed.
` + prompt, nil
	}
	if format != Express {
		return "", fmt.Errorf("unknown inference format %q", format)
	}
	_, definitions, _ := strings.Cut(prompt, "Only use the components and functions in the negotiated catalogs below.")
	return `Write prose normally. Put Express UI inside <a2ui> and </a2ui>.
Begin a surface with surface("surface_id"). Declare its root with root = Component(...).
Reference other component variables by identifier. Use Component(property=value) for named properties.
Bind data with $/path and assign data with $/path = value. Emit an event with Event("name", {key: $/path}).
Use _template($/items, child) for a repeated child and deleteSurface("surface_id") to remove a surface.
Use only the following negotiated catalogs.` + definitions, nil
}

// Decompile returns model-facing source for the supported inference format.
func Decompile(format Format, messages []a2ui.AgentMessage, catalogs ...*a2ui.Catalog) (string, error) {
	if format == DirectJSON || format == "" {
		data, err := json.MarshalIndent(messages, "", "  ")
		return string(data), err
	}
	if format != Express {
		return "", fmt.Errorf("unknown inference format %q", format)
	}
	var out strings.Builder
	printer := expressPrinter{catalogs: catalogs}
	for i, message := range messages {
		if i > 0 && message.Data != nil && message.Data.Path == "" && messages[i-1].Create != nil && messages[i-1].Create.ID == message.Data.ID {
			value, err := decodeExpressJSON(message.Data.Value)
			if err != nil {
				return "", err
			}
			printer.writeData(&out, "", value)
			continue
		}
		switch {
		case message.Create != nil:
			c := message.Create
			fmt.Fprintf(&out, "surface(%q", c.ID)
			printer.surfaceCatalog = c.Catalog
			if c.Catalog != "" && !(len(catalogs) == 1 && catalogs[0].ID == c.Catalog) {
				fmt.Fprintf(&out, ", catalogId=%q", c.Catalog)
			}
			if c.SendDataModel {
				out.WriteString(", sendDataModel=true")
			}
			if c.Metadata != nil && c.Metadata.Extensions != nil {
				fmt.Fprintf(&out, ", extensions=%s", printer.value(c.Metadata.Extensions))
			}
			out.WriteString(")\n")
			if len(c.DataModel) > 0 {
				printer.writeData(&out, "", c.DataModel)
			} else if c.DataModel != nil {
				out.WriteString("$ = {}\n")
			}
			if err := printer.writeComponents(&out, c.Components); err != nil {
				return "", err
			}
		case message.Components != nil:
			if i > 0 && messages[i-1].Create != nil && messages[i-1].Create.ID == message.Components.ID && len(messages[i-1].Create.Components) == 0 {
				if err := printer.writeComponents(&out, message.Components.Components); err != nil {
					return "", err
				}
				continue
			}
			if slices.ContainsFunc(message.Components.Components, func(component a2ui.Component) bool { return component.ID == a2ui.Root }) {
				fmt.Fprintf(&out, "surface(%q, update=true)\n", message.Components.ID)
			} else {
				fmt.Fprintf(&out, "surface(%q)\n", message.Components.ID)
			}
			if err := printer.writeComponents(&out, message.Components.Components); err != nil {
				return "", err
			}
		case message.Data != nil:
			if strings.ContainsAny(message.Data.Path, " \r\n") {
				return "", fmt.Errorf("data path cannot be represented in Express")
			}
			fmt.Fprintf(&out, "surface(%q)\n", message.Data.ID)
			value, err := decodeExpressJSON(message.Data.Value)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&out, "$%s = %s\n", message.Data.Path, printer.value(value))
		case message.Delete != nil:
			fmt.Fprintf(&out, "deleteSurface(%q)\n", message.Delete.ID)
		case message.CallRenderer != nil:
			c := message.CallRenderer
			if c.Surface != "" {
				fmt.Fprintf(&out, "surface(%q, update=true)\n", c.Surface)
			}
			out.WriteString(printer.function(c.Function.Name, c.Function.Catalog, c.Function.Args) + "\n")
		default:
			return "", fmt.Errorf("message cannot be represented in Express")
		}
	}
	return strings.TrimSpace(out.String()), nil
}

func (printer expressPrinter) writeComponents(out *strings.Builder, components []a2ui.Component) error {
	for _, component := range components {
		if !validIdentifier(component.ID) || !validIdentifier(component.Type) {
			return fmt.Errorf("component %q cannot be represented in Express", component.ID)
		}
		catalogID := component.Catalog
		schema, count := printer.lookup(component.Type, catalogID, true)
		if catalogID == "" && component.Props["catalogId"] != nil && !schemaFieldsContain(schema, "catalogId") {
			_ = json.Unmarshal(component.Props["catalogId"], &catalogID)
			schema, count = printer.lookup(component.Type, catalogID, true)
		}
		fields := make(map[string]any, len(component.Props))
		var checks any
		for _, name := range slices.Sorted(maps.Keys(component.Props)) {
			value, err := decodeExpressJSON(component.Props[name])
			if err != nil {
				return err
			}
			if err := validateExpressValue(value); err != nil {
				return err
			}
			if name == "@checks" || name == "checks" {
				checks = value
				continue
			}
			if name == "catalogId" && catalogID != "" {
				if component.Catalog == "" {
					continue
				}
				name = "componentCatalogId"
			}
			if name == "@metadata" || (name == "metadata" && schemaFieldsContain(schema, "metadata") == false) {
				metadata, _ := value.(map[string]any)
				name, value = "extensions", metadata["extensions"]
			}
			if !validIdentifier(name) {
				return fmt.Errorf("property %q cannot be represented in Express", name)
			}
			if name == "child" || name == "trigger" || name == "content" {
				if id, ok := value.(string); ok && validIdentifier(id) {
					value = reference(id)
				}
			}
			if name == "children" {
				if items, ok := value.([]any); ok {
					for i, item := range items {
						if id, ok := item.(string); ok && validIdentifier(id) {
							items[i] = reference(id)
						}
					}
				}
			}
			fields[name] = value
		}
		if catalogID != "" && (count != 1 || fields["componentCatalogId"] != nil || printer.surfaceCatalog != "" && printer.surfaceCatalog != catalogID) {
			fields["catalogId"] = catalogID
		}
		positional := slices.DeleteFunc(schemaFields(schema, "component"), func(name string) bool { return name == "catalogId" })
		if checks != nil {
			if shorthand, ok := printer.checks(checks, fields["value"]); ok {
				args := append(printer.arguments(fields, positional), shorthand)
				fmt.Fprintf(out, "%s = %s(%s)\n", component.ID, component.Type, strings.Join(args, ", "))
				continue
			}
			fields["checks"] = checks
		}
		fmt.Fprintf(out, "%s = %s(%s)\n", component.ID, component.Type, strings.Join(printer.arguments(fields, positional), ", "))
	}
	return nil
}

func decodeExpressJSON(raw []byte) (any, error) {
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	err := d.Decode(&value)
	return value, err
}

func (printer expressPrinter) value(value any) string {
	switch v := value.(type) {
	case reference:
		return string(v)
	case map[string]any:
		if path, ok := v["path"].(string); ok && len(v) == 2 {
			if component, ok := v["componentId"].(string); ok {
				return "_template($" + path + ", " + component + ")"
			}
		}
		if path, ok := v["@path"].(string); ok && len(v) == 1 {
			return "$" + path
		}
		if name, ok := v["@call"].(string); ok {
			args, _ := v["args"].(map[string]any)
			catalog, _ := v["catalogId"].(string)
			return printer.function(name, catalog, args)
		}
		if call, ok := v["functionCall"].(map[string]any); ok && len(v) == 1 {
			return printer.value(call)
		}
		if event, ok := v["event"].(map[string]any); ok && len(v) == 1 {
			return "Event(" + strings.Join(printer.arguments(event, []string{"name", "context"}), ", ") + ")"
		}
		var fields []string
		for _, name := range slices.Sorted(maps.Keys(v)) {
			key := name
			if !validIdentifier(name) {
				key = strconv.Quote(name)
			}
			fields = append(fields, key+": "+printer.value(v[name]))
		}
		return "{" + strings.Join(fields, ", ") + "}"
	case string:
		if strings.Contains(v, "\\") {
			if strings.Contains(v, "\n") && !strings.Contains(v, `"""`) {
				return `r"""` + v + `"""`
			}
			if !strings.ContainsAny(v, "\r\n\"") {
				return `r"` + v + `"`
			}
		}
		data, _ := json.Marshal(v)
		return string(data)
	case []any:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = printer.value(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "null"
		}
		return string(data)
	}
}

func (printer expressPrinter) function(name, catalog string, args map[string]any) string {
	schema, count := printer.lookup(name, catalog, false)
	if name == "@index" {
		return name + "(" + strings.Join(printer.arguments(args, []string{"offset"}), ", ") + ")"
	}
	if catalog != "" && (count > 1 || count == 0 || printer.surfaceCatalog != "" && printer.surfaceCatalog != catalog) {
		args = maps.Clone(args)
		if args == nil {
			args = make(map[string]any)
		}
		args["catalogId"] = catalog
	}
	return name + "(" + strings.Join(printer.arguments(args, schemaFields(functionArgs(schema), "")), ", ") + ")"
}

type expressPrinter struct {
	catalogs       []*a2ui.Catalog
	surfaceCatalog string
}

func (printer expressPrinter) lookup(name, id string, component bool) (json.RawMessage, int) {
	var schema json.RawMessage
	var count int
	for _, catalog := range printer.catalogs {
		definitions := catalog.Functions
		if component {
			definitions = catalog.Components
		}
		if raw := definitions[name]; raw != nil {
			count++
			if catalog.ID == id || id == "" && (catalog.ID == printer.surfaceCatalog || schema == nil) {
				schema = raw
			}
		}
	}
	return schema, count
}

func (printer expressPrinter) arguments(values map[string]any, positional []string) []string {
	var args []string
	used := make(map[string]bool)
	for _, name := range positional {
		value, exists := values[name]
		if !exists {
			break
		}
		used[name] = true
		args = append(args, printer.value(value))
	}
	keys := slices.Sorted(maps.Keys(values))
	for _, name := range keys {
		if !used[name] && name != "catalogId" {
			args = append(args, name+"="+printer.value(values[name]))
		}
	}
	if value, exists := values["catalogId"]; exists && !used["catalogId"] {
		args = append(args, "catalogId="+printer.value(value))
	}
	return args
}

func (printer expressPrinter) writeData(out *strings.Builder, path string, value any) {
	object, ok := value.(map[string]any)
	if !ok || len(object) == 0 {
		fmt.Fprintf(out, "$%s = %s\n", path, printer.value(value))
		return
	}
	for _, key := range slices.Sorted(maps.Keys(object)) {
		part := strings.NewReplacer("~", "~0", "/", "~1").Replace(key)
		if strings.ContainsAny(part, " \r\n\t=") {
			fmt.Fprintf(out, "$%s = %s\n", path, printer.value(value))
			return
		}
	}
	for _, key := range slices.Sorted(maps.Keys(object)) {
		part := strings.NewReplacer("~", "~0", "/", "~1").Replace(key)
		printer.writeData(out, path+"/"+part, object[key])
	}
}

func defaultCheckMessage(name string) string {
	if name == "" {
		return "Invalid value."
	}
	return strings.ToUpper(name[:1]) + name[1:] + " check failed"
}

func (printer expressPrinter) checks(value, field any) (string, bool) {
	checks, ok := value.([]any)
	if !ok || len(checks) == 0 {
		return "", false
	}
	var result []string
	for _, raw := range checks {
		check, _ := raw.(map[string]any)
		fn, _ := check["condition"].(map[string]any)
		name, _ := fn["@call"].(string)
		if name == "" || len(check) > 2 {
			return "", false
		}
		schema, _ := printer.lookup(name, "", false)
		order := schemaFields(functionArgs(schema), "")
		args, _ := fn["args"].(map[string]any)
		var rendered []string
		for _, key := range order {
			if v, ok := args[key]; ok {
				if key == "value" && field != nil && reflect.DeepEqual(field, v) {
					continue
				}
				rendered = append(rendered, printer.value(v))
			}
		}
		if message, _ := check["message"].(string); message != "" && message != defaultCheckMessage(name) {
			rendered = append(rendered, printer.value(message))
		}
		if catalog, _ := fn["catalogId"].(string); catalog != "" {
			rendered = append(rendered, "{catalogId: "+strconv.Quote(catalog)+"}")
		}
		if len(rendered) == 0 {
			result = append(result, "?"+name)
		} else {
			result = append(result, "?"+name+"("+strings.Join(rendered, ", ")+")")
		}
	}
	if len(result) == 1 {
		return result[0], true
	}
	return "[" + strings.Join(result, ", ") + "]", true
}

func validateExpressValue(value any) error {
	switch v := value.(type) {
	case map[string]any:
		if name, ok := v["@call"].(string); ok && strings.HasPrefix(name, "@") &&
			(name != "@index" || v["catalogId"] != nil) {
			return fmt.Errorf("invalid system function %q", name)
		}
		for _, item := range v {
			if err := validateExpressValue(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := validateExpressValue(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func schemaFieldsContain(schema json.RawMessage, name string) bool {
	return slices.Contains(schemaFields(schema, "component"), name)
}
