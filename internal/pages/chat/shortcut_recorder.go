package chat

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
)

func (app *chatApp) shortcutSettings(c *ui.Context) {
	ui.Column(c).FillWidth().Background(white).Radius(13).Border(1, border).
		Padding(12, 16).Gap(6).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, locales.DesktopShortcut()).Grow(1).MinWidth(0).FontSize(15).FontWeight(660)
			app.shortcutRecorder(c.Key("record"))
			if app.desktop.Shortcut != "" {
				label := locales.ClearDesktopShortcut()
				clear := pointerButton(c.Key("clear"), false).Label(label).Tooltip(label).
					Size(28, 28).Shrink(0).Radius(7).AlignItems(ui.Center).Justify(ui.Center)
				if clear.Hovered() {
					clear.Background(side)
				}
				clear.Children(func() { icon(c, "close", muted, 13) }).OnClick(func() {
					app.shortcutRecording = false
					app.applyDesktopShortcut("")
				})
			}
		})
		switch {
		case app.shortcutRecording:
			ui.Text(c, locales.CancelShortcutHint()).FontSize(11).TextColor(muted)
		case app.shortcutError:
			ui.Text(c, locales.DesktopShortcutFailed()).FontSize(12).TextColor(errorFG)
		case app.desktop.Shortcut != "" && app.actualShortcut == "":
			ui.Text(c, locales.DesktopShortcutUnbound()).FontSize(12).TextColor(muted)
		}
	})
}

func (app *chatApp) shortcutRecorder(c *ui.Context) {
	record := pointerButton(c, false).Label(locales.RecordDesktopShortcut()).
		Height(34).PaddingX(8).Gap(4).Shrink(0).AlignItems(ui.Center).Radius(8).
		Background(canvas).Border(1, border).FocusRingColor(accent)
	if app.shortcutRecording && !record.Focused() {
		app.shortcutRecording = false
	}
	if record.Clicked() {
		record.Focus()
		app.shortcutRecording = true
	}
	if record.Hovered() {
		record.Background(side)
	}
	record.HandleInput(app.recordShortcutInput)
	caps := shortcutCaps(app.actualShortcut, runtime.GOOS)
	if len(caps) == 0 && app.desktop.Shortcut != "" {
		caps = shortcutCaps(app.desktop.Shortcut, runtime.GOOS)
	}
	recording := app.shortcutRecording
	if recording {
		record.Background(tint).Border(1, accent).Label(locales.PressShortcutKeys())
	} else if len(caps) != 0 {
		record.Value(strings.Join(caps, " ")).Tooltip(locales.RecordDesktopShortcut())
	}
	record.Children(func() {
		switch {
		case recording:
			ui.Text(c, locales.PressShortcutKeys()).FontSize(12).TextColor(accent)
		case len(caps) == 0:
			ui.Text(c, locales.RecordDesktopShortcut()).FontSize(12).TextColor(muted)
		default:
			for _, key := range caps {
				ui.Box(c).MinWidth(22).Height(23).PaddingX(5).Center().Radius(4).
					Background(white).Border(1, border).Children(func() {
					ui.Text(c, key).FontSize(12).FontWeight(580).TextColor(ink)
				})
			}
		}
	})
}

func (app *chatApp) recordShortcutInput(event ui.InputEvent) bool {
	if !app.shortcutRecording {
		return false
	}
	switch event.Kind {
	case ui.InputKeyDown:
		if event.Repeat {
			return true
		}
		if event.Mods == 0 && event.Key == ui.KeyEscape {
			app.shortcutRecording = false
			return true
		}
		if event.Mods&^ui.Shift == 0 && event.Key == ui.KeyTab {
			app.shortcutRecording = false
			return false
		}
		if keys := shortcutAccelerator(event.Key, event.Mods); keys != "" {
			app.shortcutRecording = false
			app.applyDesktopShortcut(keys)
		}
		return true
	case ui.InputText, ui.InputCommand, ui.InputCompose:
		return true
	}
	return false
}

func shortcutAccelerator(key ui.Key, mods ui.Modifiers) string {
	isFunction := key >= ui.KeyF1 && key <= ui.KeyF12
	if !isFunction && mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 {
		return ""
	}
	var name string
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		name = string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		name = string(rune('0' + key - ui.Key0))
	case isFunction:
		name = fmt.Sprintf("F%d", key-ui.KeyF1+1)
	default:
		name = shortcutKeys[key]
	}
	if name == "" {
		return ""
	}
	var parts []string
	for _, modifier := range []struct {
		mask ui.Modifiers
		name string
	}{{ui.Super, "Super"}, {ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}} {
		if mods&modifier.mask != 0 {
			parts = append(parts, modifier.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}

var shortcutKeys = map[ui.Key]string{
	ui.KeySpace: "Space", ui.KeyEnter: "Enter", ui.KeyEscape: "Escape", ui.KeyTab: "Tab", ui.KeyBackspace: "Backspace",
	ui.KeyDelete: "Delete", ui.KeyInsert: "Insert", ui.KeyHome: "Home", ui.KeyEnd: "End",
	ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown", ui.KeyLeft: "Left", ui.KeyRight: "Right",
	ui.KeyUp: "Up", ui.KeyDown: "Down", ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",",
	ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";", ui.KeyQuote: "'",
	ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
}

func shortcutCaps(accelerator, platform string) []string {
	if accelerator == "" {
		return nil
	}
	mac := platform == "darwin"
	parts := strings.Split(accelerator, "+")
	if accelerator == "+" {
		return []string{"+"}
	}
	if strings.HasSuffix(accelerator, "++") {
		parts = append(parts[:len(parts)-2], "+")
	}
	for i, part := range parts {
		part = strings.TrimSpace(part)
		var macLabel, label string
		switch strings.ToLower(part) {
		case "cmdorctrl", "commandorcontrol":
			macLabel, label = "⌘", "Ctrl"
		case "super", "cmd", "command", "meta", "win":
			macLabel, label = "⌘", "Super"
			if platform == "windows" {
				label = "Win"
			}
		case "alt", "option", "altgr":
			macLabel, label = "⌥", "Alt"
		case "ctrl", "control":
			macLabel, label = "⌃", "Ctrl"
		case "shift":
			macLabel, label = "⇧", "Shift"
		case "up", "down", "left", "right":
			macLabel = map[string]string{"up": "↑", "down": "↓", "left": "←", "right": "→"}[strings.ToLower(part)]
			label = part
		default:
			macLabel, label = part, part
		}
		if mac {
			label = macLabel
		}
		parts[i] = label
	}
	return parts
}

func sameShortcut(a, b string) bool {
	first, second := shortcutCaps(a, runtime.GOOS), shortcutCaps(b, runtime.GOOS)
	if len(first) == 0 || len(first) != len(second) || !strings.EqualFold(first[len(first)-1], second[len(second)-1]) {
		return false
	}
	first, second = first[:len(first)-1], second[:len(second)-1]
	slices.Sort(first)
	slices.Sort(second)
	return slices.Equal(first, second)
}
