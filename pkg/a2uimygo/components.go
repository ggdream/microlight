package a2uimygo

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"strings"
	"time"

	"github.com/ggdream/microlight/pkg/a2ui"

	"github.com/egoist/mygo/ui"
)

type ComponentView func(*ui.Context, Node)

// Node is the scoped view of a component passed to custom catalog implementations.
type Node struct {
	renderer renderer
	a2ui.Component
	Scope string
	depth int
}

func (n Node) Property(key string) any        { return n.renderer.property(n.Component, n.Scope, key) }
func (n Node) Child(c *ui.Context, id string) { n.renderer.node(c, id, n.Scope, n.depth+1) }
func (n Node) Write(value any)                { n.renderer.write(n.Component, n.Scope, value) }
func (n Node) Activate(c *ui.Context)         { n.renderer.emit(c, n.ID, n.Scope) }

func (r renderer) property(node a2ui.Component, scope, key string) any {
	value, _ := r.surface.Resolve(node.Props[key], scope)
	return value
}

func (r renderer) decorate(c *ui.Context, node a2ui.Component, scope string, element ui.Element, fallbackLabel string) ui.Element {
	if raw := node.Props["@accessibility"]; raw != nil {
		value, _ := r.surface.Resolve(raw, scope)
		if access, ok := value.(map[string]any); ok {
			label := a2ui.Text(access["label"])
			if label != "" {
				element.Label(label)
			} else {
				label = fallbackLabel
			}
			if description := a2ui.Text(access["description"]); description != "" {
				element.Description(description)
			}
			if a2ui.Boolean(access["hidden"]) {
				element.AccessHidden(true)
			} else if label != "" {
				switch a2ui.Text(access["live"]) {
				case "polite":
					element.Role(ui.RoleStatus).Label(label)
				case "assertive":
					element.Role(ui.RoleAlert).Label(label)
				}
			}
		}
	}
	if weight := a2ui.Number(r.property(node, scope, "weight")); weight > 0 && weight <= 10000 {
		element.Grow(float32(weight)).MinWidth(0)
	}
	if r.StyleComponent != nil {
		r.StyleComponent(node, element)
	}
	return element
}

func (r renderer) fallback(c *ui.Context, err error) {
	if r.Fallback != nil {
		r.Fallback(c, err)
	}
}

func alignment(value string, fallback ui.Align) ui.Align {
	switch value {
	case "start":
		return ui.Start
	case "center":
		return ui.Center
	case "end":
		return ui.End
	case "stretch":
		return ui.Stretch
	case "spaceBetween":
		return ui.SpaceBetween
	case "spaceAround":
		return ui.SpaceAround
	case "spaceEvenly":
		return ui.SpaceEvenly
	default:
		return fallback
	}
}

func (r renderer) disabled(node a2ui.Component, scope string) bool {
	_, bound := a2ui.BindingPath(node.Props["value"], scope)
	return r.Disabled || !bound
}

func (r renderer) checkHint(c *ui.Context, node a2ui.Component, scope string) {
	for i, result := range r.surface.Checks(node, scope) {
		color := r.Style.Muted
		switch result.Severity {
		case "error":
			color = ui.Hex("#b42318")
		case "warning":
			color = ui.Hex("#9a6700")
		}
		key := struct {
			ID, Scope string
			Check     int
		}{node.ID, scope, i}
		ui.Text(c.Key(key), result.Message).FontSize(11).TextColor(color)
	}
}

func (r renderer) slider(c *ui.Context, node a2ui.Component, scope string) {
	low := a2ui.Number(r.property(node, scope, "min"))
	if math.IsNaN(low) {
		low = 0
	}
	high := a2ui.Number(r.property(node, scope, "max"))
	if high <= low || math.IsNaN(high) {
		r.fallback(c, fmt.Errorf("invalid A2UI slider range"))
		return
	}
	value := a2ui.Number(r.property(node, scope, "value"))
	if math.IsNaN(value) {
		value = low
	}
	value = min(high, max(low, value))
	label := r.surface.String(node.Props["label"], scope)
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		if label != "" {
			ui.Text(c, label).TextColor(r.Style.Text)
		}
		step := a2ui.Number(r.property(node, scope, "steps"))
		var control ui.Element
		if step > 0 {
			control = ui.StepSlider(c.Key("slider"), &value, low, high, (high-low)/step)
		} else {
			control = ui.Slider(c.Key("slider"), &value, low, high)
		}
		r.decorate(c, node, scope, control.Label(label).Disabled(r.disabled(node, scope)), label)
		if control.Changed() {
			r.write(node, scope, value)
		}
		r.checkHint(c, node, scope)
	})
}

func parseDate(value string) time.Time {
	for _, format := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", time.DateOnly, "15:04:05", "15:04"} {
		if parsed, err := time.ParseInLocation(format, value, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func (r renderer) dateTime(c *ui.Context, node a2ui.Component, scope string) {
	useTime := a2ui.Boolean(r.property(node, scope, "enableTime"))
	useDate := !useTime || a2ui.Boolean(r.property(node, scope, "enableDate"))
	original := r.surface.String(node.Props["value"], scope)
	value := parseDate(original)
	label := r.surface.String(node.Props["label"], scope)
	disabled := r.disabled(node, scope)
	ui.Column(c).Gap(4).Children(func() {
		if label != "" {
			ui.Text(c, label).TextColor(r.Style.Text)
		}
		changed := false
		ui.Row(c).Gap(6).Children(func() {
			if useDate {
				date := r.decorate(c, node, scope, ui.DateInput(c.Key("date"), &value).Label(label).Disabled(disabled), label)
				changed = date.Changed()
			}
			if useTime {
				tm := r.decorate(c, node, scope, ui.TimeInput(c.Key("time"), &value).Label(label).Disabled(disabled), label)
				changed = tm.Changed() || changed
			}
		})
		if changed && !disabled {
			layout := time.DateOnly
			if useTime && useDate {
				layout = "2006-01-02T15:04:05"
			} else if useTime {
				layout = "15:04:05"
			}
			if useDate && useTime && strings.ContainsAny(strings.TrimPrefix(original, strings.SplitN(original, "T", 2)[0]), "Z+-") {
				layout = time.RFC3339
				value = value.UTC()
			}
			minimum := parseDate(r.surface.String(node.Props["min"], scope))
			maximum := parseDate(r.surface.String(node.Props["max"], scope))
			if !minimum.IsZero() && value.Before(minimum) {
				value = minimum
			}
			if !maximum.IsZero() && value.After(maximum) {
				value = maximum
			}
			r.write(node, scope, value.Format(layout))
		}
		r.checkHint(c, node, scope)
	})
}

func (r renderer) tabs(c *ui.Context, node a2ui.Component, scope string, depth int) {
	var tabs []struct {
		Title json.RawMessage
		Child string
	}
	if json.Unmarshal(node.Props["tabs"], &tabs) != nil || len(tabs) == 0 {
		return
	}
	column := r.decorate(c, node, scope, ui.Column(c).FillWidth().Gap(8), "")
	selected := ui.Local(column, "selected", func() int { return 0 })
	*selected = max(0, min(*selected, len(tabs)-1))
	labels := make([]string, len(tabs))
	for i, tab := range tabs {
		labels[i] = r.surface.String(tab.Title, scope)
	}
	column.Children(func() {
		ui.Tabs(c.Key("tab-list"), selected, labels...).Disabled(r.Disabled)
		r.node(c, tabs[*selected].Child, scope, depth+1)
	})
}

func (r renderer) modal(c *ui.Context, node a2ui.Component, scope string, depth int) {
	trigger := r.surface.String(node.Props["trigger"], scope)
	content := r.surface.String(node.Props["content"], scope)
	triggerNode, hasTrigger := r.surface.Component(trigger)
	label := "Open"
	if hasTrigger && triggerNode.Type == "Button" {
		child := r.surface.String(triggerNode.Props["child"], scope)
		if text, ok := r.surface.Component(child); ok {
			if name := r.surface.String(text.Props["text"], scope); name != "" {
				label = name
			}
		}
	}
	disabled := r.Disabled || (hasTrigger && !r.surface.IsValid(triggerNode, scope))
	button := r.decorate(c, node, scope, ui.ButtonBase(c.Key("trigger")).Label(label).Disabled(disabled), label)
	open := ui.Local(button, "open", func() bool { return false })
	// The modal trigger can itself be a Button. Render its child inside
	// the modal opener so there is only one native click target.
	triggerChild := trigger
	if hasTrigger && triggerNode.Type == "Button" {
		triggerChild = r.surface.String(triggerNode.Props["child"], scope)
	}
	button.Children(func() {
		if hasTrigger && triggerNode.Type == "Button" && label != "Open" {
			ui.Text(c, label)
		} else {
			r.node(c, triggerChild, scope, depth+1)
		}
	}).OnClick(func() {
		*open = true
		if hasTrigger && triggerNode.Props["action"] != nil {
			r.emit(c, trigger, scope)
		}
	})
	ui.DialogBase(c.Key("dialog"), open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .35))
		panel.Background(r.Style.Surface).Padding(16).Radius(10).Gap(8).MinWidth(220).MaxWidth(560).
			Children(func() {
				r.node(c, content, scope, depth+1)
				ui.ButtonBase(c.Key("dismiss")).Label("Close").Children(func() { ui.Text(c, "Close") }).OnClick(func() { *open = false })
			})
	})
}

func (r renderer) icon(c *ui.Context, node a2ui.Component, scope string) {
	value := r.property(node, scope, "name")
	if object, ok := value.(map[string]any); ok {
		path := a2ui.Text(object["svgPath"])
		if len(path) <= 8192 {
			source := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="` + html.EscapeString(path) + `"/></svg>`
			if svg, err := ui.ParseSVG([]byte(source)); err == nil {
				r.decorate(c, node, scope, ui.Icon(c, svg).Size(20, 20).TextColor(r.Style.Text), "")
				return
			}
		}
	}
	name := a2ui.Text(value)
	if r.Media.Icon != nil {
		if svg := r.Media.Icon(name); svg != nil {
			r.decorate(c, node, scope, ui.Icon(c, svg).Size(20, 20).TextColor(r.Style.Text), name)
			return
		}
	}
	glyph := map[string]string{"add": "+", "check": "✓", "close": "×", "star": "★", "favorite": "♥", "arrowBack": "←", "arrowForward": "→", "moreHoriz": "⋯", "search": "⌕", "warning": "⚠", "playArrow": "▶", "pause": "Ⅱ", "info": "ⓘ"}[name]
	if glyph == "" {
		glyph = strings.ToUpper(name[:min(1, len(name))])
	}
	r.decorate(c, node, scope, ui.Text(c, glyph).Label(name).FontSize(17).TextColor(r.Style.Text), name)
}
