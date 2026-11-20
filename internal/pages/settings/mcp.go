package settings

import (
	"encoding/json"
	"github.com/ggdream/microlight/pkg/viewport"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/elastic"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

type MCPForm struct {
	name, endpoint, command, args string
	transport                     mcptools.Transport
	initialTransport              mcptools.Transport
	timeoutSeconds                uint32
	adding, revealEditor          bool
	mcpError                      string
	mcpEditing                    string
	headers                       []*mcpHeaderDraft
}

func (form *MCPForm) HasChanges(servers []mcptools.Server) bool {
	saved := MCPForm{transport: form.initialTransport}
	for _, server := range servers {
		if server.Name == form.mcpEditing {
			saved.edit(server)
			break
		}
	}
	transport := func(kind mcptools.Transport) mcptools.Transport {
		if kind == "" {
			return mcptools.HTTP
		}
		return kind
	}
	return form.name != saved.name || form.endpoint != saved.endpoint ||
		form.command != saved.command || form.args != saved.args ||
		form.timeoutSeconds != saved.timeoutSeconds ||
		transport(form.transport) != transport(saved.transport) ||
		!slices.EqualFunc(form.headers, saved.headers, func(a, b *mcpHeaderDraft) bool { return *a == *b })
}

func (form *MCPForm) View(c *ui.Context, icons asseticon.Set, servers []mcptools.Server, save func([]mcptools.Server) error, navigate func(func())) {
	ui.Column(c).FillWidth().Gap(settingGap(c, 12)).Children(func() {
		ui.Column(c).Gap(settingGap(c, 4)).Children(func() {
			ui.Text(c, "MCP").FontSize(viewport.Font(c, 18)).FontWeight(660)
			hint := locales.McpSettingsHint()
			if !desktop.Desktop {
				hint = locales.McpMobileSettingsHint()
			}
			ui.Text(c, hint).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
		})
		form.serverList(c.Key("servers"), icons, servers, save, navigate)
		if form.mcpError != "" && !form.adding && form.mcpEditing == "" {
			ui.Text(c, form.mcpError).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
		}
		if !form.adding && form.mcpEditing == "" {
			return
		}
		title, submit := locales.AddMCPServer(), locales.AddMCPServer()
		glyph := "plus"
		if form.mcpEditing != "" {
			title, submit, glyph = locales.EditMcpServer(), locales.SaveSettings(), "check"
		}
		editor := Card(c.Key("editor"), title, func() {
			Field(c, locales.McpServerName(), "my-server", &form.name)
			form.transportControl(c, icons)
			form.timeoutControl(c, icons)
			if form.transport == mcptools.Stdio {
				Field(c, locales.McpCommand(), "/opt/homebrew/bin/uvx", &form.command)
				Field(c, locales.McpArgs(), `["my-mcp-server"]`, &form.args)
			} else {
				Field(c, "URL", "https://example.com/mcp", &form.endpoint).
					Keyboard(ui.KeyboardURL).Capitalize(ui.CapitalizeNone).Autocorrect(false)
				form.headersControl(c, icons)
			}
			ui.Row(c).FillWidth().Wrap().Gap(settingGap(c, 8)).Children(func() {
				ActionButton(c.Key("submit"), icons, submit, glyph, false, material3.FilledButton).OnClick(func() { form.submit(servers, save) })
				ActionButton(c.Key("cancel"), icons, locales.Cancel(), "close", false, material3.TextButton).OnClick(form.reset)
			})
			if form.mcpError != "" {
				ui.Text(c, form.mcpError).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
			}
		})
		if form.revealEditor {
			editor.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollStart})
			form.revealEditor = false
		}
	})
}

func (form *MCPForm) transportControl(c *ui.Context, icons asseticon.Set) {
	selected := form.transport
	if selected == "" {
		selected = mcptools.HTTP
	}
	options := []mcptools.Transport{mcptools.HTTP}
	if desktop.Desktop || selected == mcptools.Stdio {
		options = append(options, mcptools.Stdio)
	}
	settingsSelect(c.Key("mcp-transport"), icons, locales.McpTransport(), &selected,
		options, mcptools.Transport.Label, func() {
			form.transport, form.mcpError = selected, ""
		})
}

func (form *MCPForm) timeoutControl(c *ui.Context, icons asseticon.Set) {
	selected := form.timeoutSeconds
	options := []uint32{0, 30, 60, 180, 300, 600}
	if !slices.Contains(options, selected) {
		options = append(options, selected)
		slices.Sort(options)
	}
	settingsSelect(c.Key("mcp-timeout"), icons, locales.McpToolTimeout(), &selected,
		options, mcpTimeoutLabel, func() { form.timeoutSeconds, form.mcpError = selected, "" })
}

func mcpTimeoutLabel(seconds uint32) string {
	if seconds == 0 {
		return locales.McpNoTimeout()
	}
	if seconds%60 == 0 {
		return locales.McpTimeoutMinutes(int(seconds / 60))
	}
	return locales.DurationSeconds(int(seconds))
}

func settingsSelect[T comparable](c *ui.Context, icons asseticon.Set, label string, selected *T, options []T, optionLabel func(T) string, change func()) {
	layout := settingsLayout(c)
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(settingGap(c, 10)).Children(func() {
		ui.Text(c, label).FontSize(viewport.Font(c, 12)).FontWeight(610)
		dropdown := ui.SelectBase(c.Key("select"), selected)
		material3.SelectFrame(dropdown.Trigger.Label(label).Value(optionLabel(*selected)).Width(layout.X(168)).MaxWidthPercent(100).Height(SelectHeight(c)).PaddingX(settingX(c, 10)).
			AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(settingGap(c, 6))).Radius(material3.ShapeSmall).
			Background(material3.Monochrome.SurfaceContainer).Cursor(ui.CursorPointer).FocusRingColor(theme.Accent).
			Children(func() {
				ui.Text(c, optionLabel(*selected)).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13))
				ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Shrink(0).Rotate(90).TextColor(theme.Muted)
			}).OnChange(change)
		dropdown.Popup(func(panel ui.Element) {
			padding := settingY(c, 4)
			material3.Menu(panel.Padding(padding, settingX(c, 4))).Radius(material3.ShapeMedium)
			room := layout.FitHeight(480, 8)
			anchor := dropdown.Trigger.LayoutBounds()
			if anchor.H > 0 {
				top, _, _, _ := c.SafeArea()
				above := anchor.Y - top
				below := top + layout.Height - anchor.Y - anchor.H
				optionHeight := layout.Target(28) + settingGap(c, 2)
				if above > below && below < float32(len(options))*optionHeight+2*padding {
					panel.AttachTo(dropdown.Trigger, ui.AnchorTopStart, ui.AnchorBottomStart)
				}
				room = min(room, max(above, below)-8)
			}
			scroll, offset := elastic.Scroll(c.Key("options"))
			scroll.Shrink(1).MinHeight(0).MaxHeight(max(1, room-2*padding-2)).FillWidth().Children(func() {
				offset.Apply(ui.Column(c)).FillWidth().Gap(settingGap(c, 2)).Children(func() {
					for _, option := range options {
						value := optionLabel(option)
						item := dropdown.Item(option).Label(value).FillWidth().MinHeight(layout.Target(28)).AlignItems(ui.Center).
							Padding(settingY(c, 6), settingX(c, 8)).Radius(5).Cursor(ui.CursorPointer)
						if item.Highlighted() {
							item.Background(theme.Hover)
						}
						item.Children(func() {
							ui.Text(c, value).MinWidth(0).FontSize(viewport.Font(c, 13))
						})
					}
				})
			})
		})
	})
}

func (form *MCPForm) submit(existing []mcptools.Server, save func([]mcptools.Server) error) {
	server := mcptools.Server{Name: strings.TrimSpace(form.name), Transport: form.transport, TimeoutSeconds: form.timeoutSeconds}
	server.Transport = server.Kind()
	if server.Transport == mcptools.Stdio {
		server.Command = strings.TrimSpace(form.command)
		if raw := strings.TrimSpace(form.args); raw != "" {
			var args []*string
			if json.Unmarshal([]byte(raw), &args) != nil || args == nil || slices.Contains(args, nil) {
				form.mcpError = locales.InvalidMcpArgs()
				return
			}
			for _, arg := range args {
				server.Args = append(server.Args, *arg)
			}
		}
	} else {
		server.URL = strings.TrimSpace(form.endpoint)
		for _, header := range form.headers {
			server.Headers = append(server.Headers, header.requestHeader())
		}
	}
	servers := slices.Clone(existing)
	i := slices.IndexFunc(servers, func(saved mcptools.Server) bool { return saved.Name == form.mcpEditing })
	if i >= 0 {
		servers[i] = server
	} else {
		servers = append(servers, server)
	}
	if form.save(save, servers) {
		form.reset()
	}
}

func (form *MCPForm) edit(server mcptools.Server) {
	form.reset()
	form.mcpEditing, form.name = server.Name, server.Name
	form.transport, form.endpoint, form.command = server.Kind(), server.URL, server.Command
	form.timeoutSeconds = server.TimeoutSeconds
	if len(server.Args) != 0 {
		args, _ := json.Marshal(server.Args)
		form.args = string(args)
	}
	for _, header := range server.Headers {
		form.headers = append(form.headers, editMCPHeader(header))
	}
}

func (form *MCPForm) reset() {
	form.mcpEditing, form.mcpError = "", ""
	form.name, form.endpoint, form.command, form.args = "", "", "", ""
	form.headers = nil
	form.timeoutSeconds = 0
	form.adding, form.revealEditor = false, false
	form.initialTransport = form.transport
}

func (form *MCPForm) save(apply func([]mcptools.Server) error, servers []mcptools.Server) bool {
	if err := apply(servers); err != nil {
		form.mcpError = err.Error()
		return false
	}
	form.mcpError = ""
	return true
}
