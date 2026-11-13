// Package a2uimygo renders A2UI surfaces using native MyGo views.
package a2uimygo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/pkg/a2ui"

	"github.com/egoist/mygo/ui"
)

type Style struct {
	Text, Muted, Border, Surface, Accent, Tint, Caret ui.Color
	ControlHeight                                     float32
}

// Options configures the host's appearance and receives user events.
type Options struct {
	StyleComponent func(a2ui.Component, ui.Element)
	Components     map[string]map[string]ComponentView
	Media          MediaOptions
	Fallback       func(*ui.Context, error)
	OpenURL        func(string) error
	Style          Style
	Disabled       bool
	OnAction       func(a2ui.RendererMessage)
	OnError        func(error)
	CanActivate    func(a2ui.RendererMessage) bool
	EnterActions   map[string]string
	// After renders host controls beside a component.
	After map[string]func(*ui.Context)
}

func View(c *ui.Context, surface *a2ui.Surface, options Options) {
	if surface == nil {
		return
	}
	theme := c.Theme()
	for color, fallback := range map[*ui.Color]ui.Color{
		&options.Style.Text: theme.Text, &options.Style.Muted: theme.TextMuted,
		&options.Style.Border: theme.Border, &options.Style.Surface: theme.Surface,
		&options.Style.Accent: theme.Accent, &options.Style.Tint: theme.Background,
		&options.Style.Caret: theme.Text,
	} {
		if color.A == 0 {
			*color = fallback
		}
	}
	if options.Style.ControlHeight == 0 {
		options.Style.ControlHeight = 34
	}
	renderer := renderer{surface: surface, Options: options}
	renderer.node(c, a2ui.Root, "", 0)
}

type renderer struct {
	surface *a2ui.Surface
	Options
}

func (r renderer) node(c *ui.Context, id, scope string, depth int) {
	node, ok := r.surface.Component(id)
	if !ok || depth >= 128 {
		if depth >= 128 {
			r.fallback(c, fmt.Errorf("A2UI view depth exceeded"))
		}
		return
	}
	catalog := node.Catalog
	if catalog == "" {
		catalog = r.surface.Catalog()
	}
	c = c.Key(struct{ Surface, ID, Scope, Type, Catalog string }{r.surface.ID(), id, scope, node.Type, catalog})
	if view := r.Components[catalog][node.Type]; view != nil {
		view(c, Node{r, node, scope, depth})
		return
	}
	if catalog != "" && catalog != a2ui.BasicCatalog {
		r.fallback(c, fmt.Errorf("unimplemented component %q in catalog %q", node.Type, catalog))
		return
	}
	prop := func(key string) string { return r.surface.String(node.Props[key], scope) }
	children := func() { r.children(c, node.Props["children"], scope, depth+1) }
	switch node.Type {
	case "Text":
		r.text(c, node, scope)
	case "Column":
		r.decorate(c, node, scope, ui.Column(c).FillWidth().Gap(8).AlignItems(alignment(prop("align"), ui.Stretch)).Justify(alignment(prop("justify"), ui.Start)), "").Children(children)
	case "Row":
		r.decorate(c, node, scope, ui.Row(c).FillWidth().Gap(8).AlignItems(alignment(prop("align"), ui.Center)).Justify(alignment(prop("justify"), ui.Start)), "").Children(children)
	case "Card":
		r.decorate(c, node, scope, ui.Column(c).FillWidth().Padding(12).Radius(10).Background(r.Style.Surface).Border(1, r.Style.Border), "").Children(func() { r.node(c, prop("child"), scope, depth+1) })
	case "Image", "Video", "AudioPlayer":
		r.media(c, node, scope)
	case "Icon":
		r.icon(c, node, scope)
	case "Tabs":
		r.tabs(c, node, scope, depth)
	case "Modal":
		r.modal(c, node, scope, depth)
	case "Slider":
		r.slider(c, node, scope)
	case "DateTimeInput":
		r.dateTime(c, node, scope)
	case "List":
		align := alignment(prop("align"), ui.Start)
		if prop("direction") == "horizontal" {
			r.decorate(c, node, scope, ui.ScrollHorizontal(c).FillWidth(), "").Children(func() { ui.Row(c).AlignItems(align).Gap(8).Children(children) })
		} else {
			r.decorate(c, node, scope, ui.Scroll(c).FillWidth().MaxHeight(280), "").Children(func() { ui.Column(c).FillWidth().AlignItems(align).Gap(8).Children(children) })
		}
	case "Divider":
		line := ui.Box(c).Background(r.Style.Border)
		if prop("axis") == "vertical" {
			line.Width(1).Height(r.Style.ControlHeight)
		} else {
			line.FillWidth().Height(1)
		}
		r.decorate(c, node, scope, line, "")
	case "TextField":
		r.textField(c, node, scope)
	case "Button":
		r.button(c, node, scope, depth)
	case "CheckBox":
		value, _ := r.surface.Resolve(node.Props["value"], scope)
		checked := a2ui.Boolean(value)
		check := ui.Checkbox(c, &checked, prop("label")).Disabled(r.disabled(node, scope))
		r.decorate(c, node, scope, check, prop("label"))
		if check.Changed() {
			r.write(node, scope, checked)
		}
		r.checkHint(c, node, scope)
	case "ChoicePicker":
		r.choicePicker(c, node, scope)
	default:
		r.fallback(c, fmt.Errorf("unimplemented component %q", node.Type))
	}
	if after := r.After[id]; after != nil {
		after(c.Key("host"))
	}
}

func (r renderer) children(c *ui.Context, raw json.RawMessage, scope string, depth int) {
	var ids []string
	if json.Unmarshal(raw, &ids) == nil {
		for _, id := range ids {
			r.node(c, id, scope, depth)
		}
		return
	}
	var template struct {
		Path      string `json:"path"`
		Component string `json:"componentId"`
	}
	if json.Unmarshal(raw, &template) != nil || template.Component == "" {
		return
	}
	path := a2ui.ResolvePath(scope, template.Path)
	value, _ := r.surface.Data().Get(path)
	items, _ := value.([]any)
	for i := range items {
		itemPath := strings.TrimSuffix(path, "/") + "/" + strconv.Itoa(i)
		r.node(c, template.Component, itemPath, depth)
	}
}

func (r renderer) write(node a2ui.Component, scope string, value any) {
	path, ok := a2ui.BindingPath(node.Props["value"], scope)
	if ok {
		if err := r.surface.Data().Set(path, value); err != nil && r.OnError != nil {
			r.OnError(err)
		}
	}
}

func (r renderer) textField(c *ui.Context, node a2ui.Component, scope string) {
	value := a2ui.Text(r.property(node, scope, "value"))
	label := r.surface.String(node.Props["label"], scope)
	placeholder := r.surface.String(node.Props["placeholder"], scope)
	_, bound := a2ui.BindingPath(node.Props["value"], scope)
	var input ui.Element
	switch r.surface.String(node.Props["variant"], scope) {
	case "obscured":
		input = ui.TextInputBase(c, &value).Height(r.Style.ControlHeight).Password()
	case "longText":
		input = ui.TextAreaBase(c, &value).Height(85)
	default:
		input = ui.TextInputBase(c, &value).Height(r.Style.ControlHeight)
	}
	r.decorate(c, node, scope, input.Label(label), label).Invalid(!r.surface.IsValid(node, scope)).Placeholder(placeholder).Grow(1).MinWidth(0).PaddingX(9).
		Border(1, r.Style.Border).Radius(6).CaretColor(r.Style.Caret).Disabled(r.Disabled).
		ReadOnly(!bound).OnChange(func() {
		if r.surface.String(node.Props["variant"], scope) == "number" && value != "" {
			if number, err := strconv.ParseFloat(value, 64); err == nil {
				r.write(node, scope, number)
			}
			return
		}
		r.write(node, scope, value)
	})
	r.checkHint(c, node, scope)
	if enter := r.EnterActions[node.ID]; enter != "" {
		input.OnTextKey(func(key ui.TextKey) bool {
			if key.Key != ui.KeyEnter || key.Mods != 0 || r.Disabled {
				return false
			}
			r.write(node, scope, key.Text())
			r.emit(c, enter, scope)
			return true
		})
	}
}

func (r renderer) button(c *ui.Context, node a2ui.Component, scope string, depth int) {
	child := r.surface.String(node.Props["child"], scope)
	label := ""
	if text, ok := r.surface.Component(child); ok {
		label = r.surface.String(text.Props["text"], scope)
	}
	disabled := r.Disabled || !r.surface.IsValid(node, scope)
	if !disabled && r.CanActivate != nil {
		event, err := r.surface.Action(node.ID, scope, c.Now())
		disabled = err != nil || !r.CanActivate(event)
	}
	button := ui.ButtonBase(c).Label(label).Height(r.Style.ControlHeight).
		PaddingX(10).Shrink(0).Radius(7).AlignItems(ui.Center).Justify(ui.Center).
		Disabled(disabled).Cursor(ui.CursorPointer)
	switch r.surface.String(node.Props["variant"], scope) {
	case "primary":
		button.Background(r.Style.Tint)
	case "borderless":
	default:
		button.Border(1, r.Style.Border)
	}
	r.decorate(c, node, scope, button, label)
	if button.Hovered() && !disabled {
		button.Background(r.Style.Tint)
	}
	button.Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(12).FontWeight(620).TextColor(r.Style.Accent)
		} else {
			r.node(c, child, scope, depth+1)
		}
	}).OnClick(func() { r.emit(c, node.ID, scope) })
}

func (r renderer) emit(c *ui.Context, id, scope string) {
	if r.Disabled {
		return
	}
	if component, ok := r.surface.Component(id); !ok || !r.surface.IsValid(component, scope) {
		return
	}
	open := r.OpenURL
	if open == nil {
		open = func(url string) error { c.OpenURL(url); return nil }
	}
	event, err := r.surface.Activate(id, scope, c.Now(), open)
	if err != nil {
		if r.OnError != nil {
			r.OnError(err)
		}
		return
	}
	if event != nil && r.OnAction != nil && (r.CanActivate == nil || r.CanActivate(*event)) {
		r.OnAction(*event)
	}
}

func (r renderer) choicePicker(c *ui.Context, node a2ui.Component, scope string) {
	var choices []struct {
		Label any    `json:"label"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(node.Props["options"], &choices); err != nil {
		return
	}
	value, _ := r.surface.Resolve(node.Props["value"], scope)
	selected := make(map[string]bool)
	if values, ok := value.([]any); ok {
		for _, item := range values {
			if name, ok := item.(string); ok {
				selected[name] = true
			}
		}
	}
	single := r.surface.String(node.Props["variant"], scope) != "multipleSelection"
	disabled := r.disabled(node, scope)
	var search string
	group := ui.Column(c).Gap(4)
	if single {
		group.Role(ui.RoleRadioGroup)
	}
	container := r.decorate(c, node, scope, group, r.surface.String(node.Props["label"], scope))
	query := ui.Local(container, "search", func() string { return "" })
	container.Children(func() {
		if label := r.surface.String(node.Props["label"], scope); label != "" {
			ui.Text(c, label).TextColor(r.Style.Text)
		}
		if a2ui.Boolean(r.property(node, scope, "filterable")) {
			ui.TextInputBase(c.Key("filter"), query).Label("Filter").Placeholder("Filter")
			search = strings.ToLower(*query)
		}
		for _, choice := range choices {
			value, err := r.surface.Data().Resolve(choice.Label, scope)
			label, ok := value.(string)
			if err != nil || !ok || !strings.Contains(strings.ToLower(label), search) {
				continue
			}
			checked := selected[choice.Value]
			update := func() {
				if single && checked {
					clear(selected)
				}
				selected[choice.Value] = checked
				values := make([]any, 0, len(selected))
				for _, candidate := range choices {
					if selected[candidate.Value] {
						values = append(values, candidate.Value)
					}
				}
				r.write(node, scope, values)
			}
			if r.surface.String(node.Props["displayStyle"], scope) == "chips" {
				chip := ui.ButtonBase(c.Key(choice.Value)).Label(label).AccessSelected(checked).
					Disabled(disabled).Radius(12).Padding(4, 8).Border(1, r.Style.Border)
				if checked {
					chip.Background(r.Style.Tint)
				}
				chip.Children(func() { ui.Text(c, label) }).OnClick(func() {
					checked = !checked
					update()
				})
			} else if single {
				current := ""
				if checked {
					current = choice.Value
				}
				if ui.Radio(c.Key(choice.Value), &current, choice.Value, label).Disabled(disabled).Changed() {
					checked = true
					update()
				}
			} else if ui.Checkbox(c.Key(choice.Value), &checked, label).Disabled(disabled).Changed() {
				update()
			}
		}
		r.checkHint(c, node, scope)
	})
}
