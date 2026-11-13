package a2uiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"
)

type Format string

const (
	DirectJSON Format = "direct_json"
	Express    Format = "express"
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
func Decompile(format Format, messages []a2ui.AgentMessage) (string, error) {
	if format == DirectJSON || format == "" {
		data, err := json.MarshalIndent(messages, "", "  ")
		return string(data), err
	}
	if format != Express {
		return "", fmt.Errorf("unknown inference format %q", format)
	}
	var out strings.Builder
	for _, message := range messages {
		switch {
		case message.Create != nil:
			c := message.Create
			fmt.Fprintf(&out, "surface(%q", c.ID)
			if c.Catalog != "" {
				fmt.Fprintf(&out, ", catalogId=%q", c.Catalog)
			}
			if c.SendDataModel {
				out.WriteString(", sendDataModel=true")
			}
			if c.Metadata != nil && c.Metadata.Extensions != nil {
				fmt.Fprintf(&out, ", extensions=%s", expressValue(c.Metadata.Extensions))
			}
			out.WriteString(")\n")
			if c.DataModel != nil {
				fmt.Fprintf(&out, "$ = %s\n", expressValue(c.DataModel))
			}
			if err := writeComponents(&out, c.Components); err != nil {
				return "", err
			}
		case message.Components != nil:
			fmt.Fprintf(&out, "surface(%q, update=true)\n", message.Components.ID)
			if err := writeComponents(&out, message.Components.Components); err != nil {
				return "", err
			}
		case message.Data != nil:
			if strings.ContainsAny(message.Data.Path, " \r\n") {
				return "", fmt.Errorf("data path cannot be represented in Express")
			}
			fmt.Fprintf(&out, "surface(%q, update=true)\n", message.Data.ID)
			value, err := decodeExpressJSON(message.Data.Value)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&out, "$%s = %s\n", message.Data.Path, expressValue(value))
		case message.Delete != nil:
			fmt.Fprintf(&out, "deleteSurface(%q)\n", message.Delete.ID)
		case message.CallRenderer != nil:
			c := message.CallRenderer
			if c.Surface != "" {
				fmt.Fprintf(&out, "surface(%q, update=true)\n", c.Surface)
			}
			out.WriteString(expressFunction(c.Function.Name, c.Function.Args) + "\n")
		default:
			return "", fmt.Errorf("message cannot be represented in Express")
		}
	}
	return strings.TrimSpace(out.String()), nil
}

func writeComponents(out *strings.Builder, components []a2ui.Component) error {
	for _, component := range components {
		if !validIdentifier(component.ID) || !validIdentifier(component.Type) {
			return fmt.Errorf("component %q cannot be represented in Express", component.ID)
		}
		fields := make([]string, 0, len(component.Props))
		for _, name := range slices.Sorted(maps.Keys(component.Props)) {
			value, err := decodeExpressJSON(component.Props[name])
			if err != nil {
				return err
			}
			if name == "@checks" {
				name = "checks"
			}
			if name == "@metadata" {
				metadata, _ := value.(map[string]any)
				name, value = "extensions", metadata["extensions"]
			}
			if !validIdentifier(name) {
				return fmt.Errorf("property %q cannot be represented in Express", name)
			}
			fields = append(fields, name+"="+expressValue(value))
		}
		fmt.Fprintf(out, "%s = %s(%s)\n", component.ID, component.Type, strings.Join(fields, ", "))
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

func expressValue(value any) string {
	switch v := value.(type) {
	case map[string]any:
		if path, ok := v["@path"].(string); ok {
			return "$" + path
		}
		if name, ok := v["@call"].(string); ok {
			args, _ := v["args"].(map[string]any)
			return expressFunction(name, args)
		}
		if call, ok := v["functionCall"].(map[string]any); ok {
			return expressValue(call)
		}
		if event, ok := v["event"].(map[string]any); ok {
			var args []string
			for _, name := range []string{"name", "context", "userMessage"} {
				if value, ok := event[name]; ok {
					args = append(args, name+"="+expressValue(value))
				}
			}
			return "Event(" + strings.Join(args, ", ") + ")"
		}
		var fields []string
		for _, name := range slices.Sorted(maps.Keys(v)) {
			fields = append(fields, strconv.Quote(name)+": "+expressValue(v[name]))
		}
		return "{" + strings.Join(fields, ", ") + "}"
	case []any:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = expressValue(item)
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

func expressFunction(name string, args map[string]any) string {
	var fields []string
	for _, key := range slices.Sorted(maps.Keys(args)) {
		fields = append(fields, key+"="+expressValue(args[key]))
	}
	return name + "(" + strings.Join(fields, ", ") + ")"
}
