package settings

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

type mcpHeaderPreset uint8

const (
	mcpHeaderBearer mcpHeaderPreset = iota
	mcpHeaderAPIKey
	mcpHeaderCustom
)

func (preset mcpHeaderPreset) label() string {
	switch preset {
	case mcpHeaderBearer:
		return "Bearer Token"
	case mcpHeaderAPIKey:
		return "X-API-Key"
	default:
		return locales.McpHeaderCustom()
	}
}

type mcpHeaderDraft struct {
	preset      mcpHeaderPreset
	name, value string
}

func editMCPHeader(header mcptools.Header) *mcpHeaderDraft {
	if strings.EqualFold(header.Name, "X-API-Key") {
		return &mcpHeaderDraft{preset: mcpHeaderAPIKey, value: header.Value}
	}
	if strings.EqualFold(header.Name, "Authorization") {
		if token, ok := strings.CutPrefix(header.Value, "Bearer "); ok && strings.TrimSpace(token) != "" {
			return &mcpHeaderDraft{preset: mcpHeaderBearer, value: token}
		}
	}
	return &mcpHeaderDraft{preset: mcpHeaderCustom, name: header.Name, value: header.Value}
}

func (draft mcpHeaderDraft) requestHeader() mcptools.Header {
	switch draft.preset {
	case mcpHeaderBearer:
		value := draft.value
		if strings.TrimSpace(value) != "" {
			value = "Bearer " + value
		}
		return mcptools.Header{Name: "Authorization", Value: value}
	case mcpHeaderAPIKey:
		return mcptools.Header{Name: "X-API-Key", Value: draft.value}
	default:
		return mcptools.Header{Name: strings.TrimSpace(draft.name), Value: draft.value}
	}
}

func (form *MCPForm) headersControl(c *ui.Context, icons asseticon.Set) {
	ui.Column(c.Key("headers")).FillWidth().Gap(settingGap(c, 8)).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(settingGap(c, 6)).Children(func() {
			ui.Text(c, locales.McpHeaders()).FontSize(viewport.Font(c, 12)).FontWeight(610)
			ActionButton(c.Key("add-header"), icons, locales.AddMcpHeader(), "plus", false).OnClick(func() {
				form.headers = append(form.headers, &mcpHeaderDraft{})
				form.mcpError = ""
			})
		})
		for i, header := range form.headers {
			ui.Row(c.Key(header)).FillWidth().AlignItems(ui.Start).Gap(settingGap(c, 8)).Children(func() {
				fields := ui.Row
				if settingsLayout(c).Compact() {
					fields = ui.Column
				}
				fields(c.Key("fields")).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Gap(settingGap(c, 8)).Children(func() {
					mcpHeaderPresetControl(c, icons, header)
					ui.Column(c.Key("value")).Grow(1).MinWidth(0).Gap(settingGap(c, 8)).Children(func() {
						label := locales.McpHeaderValue()
						switch header.preset {
						case mcpHeaderBearer:
							label = locales.McpBearerToken()
						case mcpHeaderAPIKey:
							label = "API Key"
						case mcpHeaderCustom:
							Field(c, locales.McpHeaderName(), "", &header.name)
						}
						Field(c.Key("credential"), label, "", &header.value).Password()
					})
				})
				button := theme.PointerButton(c.Key("remove"), false).Label(locales.RemoveMcpHeader()).
					Tooltip(locales.RemoveMcpHeader()).Size(settingsLayout(c).Target(35), settingsLayout(c).Target(35)).Margin(settingY(c, 21), 0, 0).Shrink(0).Radius(7).
					AlignItems(ui.Center).Justify(ui.Center)
				if button.Hovered() {
					button.Background(theme.Hover)
				}
				button.Children(func() { ui.Icon(c, icons.SVG("close")).Size(14, 14).Shrink(0).TextColor(theme.Muted) }).OnClick(func() {
					form.headers = slices.Delete(form.headers, i, i+1)
					form.mcpError = ""
				})
			})
		}
	})
}

func mcpHeaderPresetControl(c *ui.Context, icons asseticon.Set, header *mcpHeaderDraft) {
	label := locales.McpHeaderType()
	group := ui.Column(c.Key("preset")).Shrink(0).Gap(settingGap(c, 5))
	if settingsLayout(c).Compact() {
		group.FillWidth()
	} else {
		group.Width(145)
	}
	group.Children(func() {
		ui.Text(c, label).FontSize(viewport.Font(c, 12)).FontWeight(610)
		dropdown := ui.SelectBase(c.Key("select"), &header.preset)
		material3.SelectFrame(dropdown.Trigger.Label(label).Value(header.preset.label()).FillWidth().Height(SelectHeight(c)).PaddingX(settingX(c, 10)).
			AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(settingGap(c, 6))).Radius(material3.ShapeSmall).
			Background(material3.Monochrome.SurfaceContainer).Cursor(ui.CursorPointer).FocusRingColor(theme.Accent).Children(func() {
			ui.Text(c, header.preset.label()).FontSize(viewport.Font(c, 13))
			ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Shrink(0).Rotate(90).TextColor(theme.Muted)
		})
		dropdown.Popup(func(panel ui.Element) {
			material3.Menu(panel.Padding(settingY(c, 4), settingX(c, 4)).Gap(settingGap(c, 2))).Radius(material3.ShapeMedium)
			for _, preset := range []mcpHeaderPreset{mcpHeaderBearer, mcpHeaderAPIKey, mcpHeaderCustom} {
				item := dropdown.Item(preset).Label(preset.label()).Padding(settingY(c, 6), settingX(c, 8)).Radius(5).Cursor(ui.CursorPointer)
				if item.Highlighted() {
					item.Background(theme.Hover)
				}
				item.Children(func() {
					ui.Text(c, preset.label()).FontSize(viewport.Font(c, 13))
				})
			}
		})
	})
}
