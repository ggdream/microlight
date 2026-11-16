package settings

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
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
	Section(c, "MCP", locales.McpSettingsHint(), func() {
		form.serverList(c.Key("servers"), icons, servers, save, navigate)
		if form.mcpError != "" && !form.adding && form.mcpEditing == "" {
			ui.Text(c, form.mcpError).FontSize(12).TextColor(theme.ErrorFG)
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
				Field(c, "URL", "https://example.com/mcp", &form.endpoint)
				form.headersControl(c, icons)
			}
			ui.Row(c).Gap(8).Children(func() {
				ActionButton(c.Key("submit"), icons, submit, glyph, false).OnClick(func() { form.submit(servers, save) })
				ActionButton(c.Key("cancel"), icons, locales.Cancel(), "close", false).OnClick(form.reset)
			})
			if form.mcpError != "" {
				ui.Text(c, form.mcpError).FontSize(12).TextColor(theme.ErrorFG)
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
	mcpSelect(c.Key("mcp-transport"), icons, locales.McpTransport(), &selected,
		[]mcptools.Transport{mcptools.HTTP, mcptools.Stdio}, mcptools.Transport.Label, func() {
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
	mcpSelect(c.Key("mcp-timeout"), icons, locales.McpToolTimeout(), &selected,
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

func mcpSelect[T comparable](c *ui.Context, icons asseticon.Set, label string, selected *T, options []T, optionLabel func(T) string, change func()) {
	ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(10).Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(610)
		dropdown := ui.SelectBase(c.Key("select"), selected)
		dropdown.Trigger.Label(label).Value(optionLabel(*selected)).Width(168).Height(35).PaddingX(10).
			AlignItems(ui.Center).Justify(ui.SpaceBetween).Border(1, theme.Border).Radius(7).
			Background(theme.Canvas).Cursor(ui.CursorPointer).FocusRingColor(theme.Accent).
			Children(func() {
				ui.Text(c, optionLabel(*selected)).FontSize(13)
				ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Rotate(90).TextColor(theme.Muted)
			}).OnChange(change)
		dropdown.Popup(func(panel ui.Element) {
			panel.Padding(4).Gap(2).Background(theme.White).Border(1, theme.Border).Radius(8)
			for _, option := range options {
				value := optionLabel(option)
				item := dropdown.Item(option).Label(value).Padding(6, 8).Radius(5).Cursor(ui.CursorPointer)
				if item.Highlighted() {
					item.Background(theme.Tint)
				}
				item.Children(func() {
					ui.Text(c, value).FontSize(13)
				})
			}
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
