package chat

import (
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

type modelOption struct {
	id, label string
}

func modelSelect(c *ui.Context, label, value string, selected *string, options []modelOption, width float32, dot ui.Color, change func()) {
	selectModel := ui.SelectBase(c, selected)
	trigger := selectModel.Trigger.Label(label).Value(value).Tooltip(value).
		Height(29).MaxWidth(width).MinWidth(0).PaddingX(9).Radius(6).Gap(6).
		AlignItems(ui.Center).Background(side).Cursor(ui.CursorPointer)
	if trigger.Hovered() {
		trigger.Background(hover)
	}
	trigger.Children(func() {
		if dot.A != 0 {
			ui.Box(c).Size(6, 6).Shrink(0).Radius(3).Background(dot)
		}
		ui.Text(c, value).MinWidth(0).SingleLine().Ellipsis("…").FontSize(11).FontWeight(620).TextColor(ink)
		ui.Icon(c, icons.SVG("ios_chevron_right")).Size(10, 10).Shrink(0).Rotate(90).TextColor(muted)
	}).OnChange(change)
	selectModel.Popup(func(panel ui.Element) {
		panel.Width(240).MaxHeight(255).Padding(4).Gap(2).Background(white).Border(1, border).Radius(8)
		for _, option := range options {
			item := selectModel.Item(option.id).Label(option.label).Padding(6, 8).
				AlignItems(ui.Center).Gap(8).Radius(5).Cursor(ui.CursorPointer)
			if item.Highlighted() || option.id == *selected {
				item.Background(tint)
			}
			item.Children(func() {
				ui.Text(c, option.label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12)
				if option.id == *selected {
					icon(c, "check", accent, 13)
				}
			})
		}
	})
}

func (app *chatApp) providerOptions() []modelOption {
	options := make([]modelOption, 0, len(app.providers.Providers))
	for _, p := range app.providers.Providers {
		options = append(options, modelOption{id: p.ID, label: p.Name})
	}
	return options
}

func (app *chatApp) chatProviderSelect(c *ui.Context) {
	id := app.providers.Selected.ProviderID
	p := app.providers.provider(id)
	if p == nil {
		return
	}
	dot := accent
	if app.backend.configErr != nil {
		dot = errorFG
	}
	modelSelect(c, locales.ModelProvider(), p.Name, &id, app.providerOptions(), 160, dot, func() {
		app.selectProvider(modelChoice{ProviderID: id})
	})
}

func (app *chatApp) chatModelSelect(c *ui.Context) {
	p := app.providers.provider(app.providers.Selected.ProviderID)
	if p == nil {
		return
	}
	model := app.settings.Model
	options := make([]modelOption, 0, len(p.Models))
	for _, name := range p.Models {
		label := name
		if name == p.Model {
			label += " · " + locales.DefaultModel()
		}
		options = append(options, modelOption{id: name, label: label})
	}
	modelSelect(c, locales.ModelID(), model, &model, options, 135, ui.Transparent, func() {
		if model == p.Model {
			model = ""
		}
		app.selectProvider(modelChoice{ProviderID: p.ID, Model: model})
	})
}

func (app *chatApp) providerSettingsControl(c *ui.Context) {
	form := &app.form
	options := app.providerOptions()
	if _, drafting := form.providerDrafts[""]; drafting || form.providerID == "" {
		options = append(options, modelOption{label: locales.NewProvider()})
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		scroll, offset := elastic.Horizontal(c.Key("provider-chips"))
		scroll.Grow(1).MinWidth(0).AlignItems(ui.Center).Padding(3, 0).Children(func() {
			offset.Apply(ui.Row(c)).Gap(8).AlignItems(ui.Center).Children(func() {
				for _, option := range options {
					app.providerChip(c.Key("provider-"+option.id), option)
				}
			})
		})
		settings.ActionButton(c.Key("new-provider"), icons, locales.AddProvider(), "plus", form.providerID == "").
			Shrink(0).Radius(17).OnClick(func() { app.editProvider("") })
	})
}

func (app *chatApp) providerModelsControl(c *ui.Context) {
	form := &app.form
	settings.Card(c, locales.ProviderModels(), func() {
		ui.Text(c, locales.DefaultModelHint()).FontSize(12).TextColor(muted)
		ui.RadioGroup(c.Key("default-model"), func() {
			for i, model := range form.models {
				row := c.Key(model)
				ui.Row(row).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					label := strings.TrimSpace(*model)
					if label == "" {
						label = locales.ModelID()
					}
					radio := ui.RadioBase(row.Key("default"), &form.defaultModel, model).
						Label(locales.ChooseDefaultModel(label)).Tooltip(locales.DefaultModel()).
						Size(25, 25).Center().Radius(6).FocusRingColor(accent).
						Cursor(ui.CursorPointer).OnChange(app.clearConnectionTest)
					radio.Children(func() {
						ring := ui.Box(row.Key("indicator")).Size(16, 16).Radius(8).Center().
							Background(white).Border(1, muted)
						if form.defaultModel == model {
							ring.Background(tint).Border(1.5, accent).Children(func() {
								ui.Box(row.Key("selected-dot")).Size(6, 6).Radius(3).Background(accent)
							})
						} else if radio.Hovered() {
							ring.Background(side).Border(1, accent)
						}
					})
					settings.TextField(row.Key("id"), locales.ModelID(), locales.ModelIDHint(), model).Grow(1).MinWidth(0).
						OnChange(app.clearConnectionTest)
					if len(form.models) > 1 {
						messageIcon(row.Key("remove"), locales.RemoveProviderModel(), "close", false, func() {
							form.models = slices.Delete(form.models, i, i+1)
							if form.defaultModel == model {
								form.defaultModel = form.models[0]
								for _, remaining := range form.models {
									if strings.TrimSpace(*remaining) != "" {
										form.defaultModel = remaining
										break
									}
								}
							}
							app.clearConnectionTest()
						})
					}
				})
			}
		}).Label(locales.DefaultModel()).Gap(8)
		settings.ActionButton(c.Key("add-model"), icons, locales.AddProviderModel(), "plus", false).
			OnClick(func() { form.models = append(form.models, new(string)) })
		app.connectionTestControl(c.Key("connection-test"))
	})
}

func (app *chatApp) providerProtocolControl(c *ui.Context) {
	form := &app.form
	family := "OpenAI"
	if form.protocol == anthropicMessages {
		family = "Anthropic"
	}
	control := func(c *ui.Context, label string, selected *string, options []modelOption, change func()) {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
			ui.Text(c, label).FontSize(12).FontWeight(610)
			modelSelect(c.Key("select"), label, *selected, selected, options, 180, ui.Transparent, change)
		})
	}
	control(c.Key("family"), locales.ModelApiFamily(), &family, []modelOption{
		{id: "OpenAI", label: "OpenAI"}, {id: "Anthropic", label: "Anthropic"},
	}, func() {
		form.protocol = openAIResponses
		if family == "Anthropic" {
			form.protocol = anthropicMessages
		}
		app.clearConnectionTest()
	})
	if form.protocol != anthropicMessages {
		route := "Responses"
		if form.protocol == openAIChat {
			route = "Chat Completions"
		}
		control(c.Key("route"), locales.ModelApiRoute(), &route, []modelOption{
			{id: "Responses", label: "Responses"}, {id: "Chat Completions", label: "Chat Completions"},
		}, func() {
			form.protocol = openAIResponses
			if route == "Chat Completions" {
				form.protocol = openAIChat
			}
			app.clearConnectionTest()
		})
	} else {
		ui.Row(c.Key("route")).FillWidth().Height(29).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
			ui.Text(c, locales.ModelApiRoute()).FontSize(12).FontWeight(610)
			ui.Text(c, "Messages").FontSize(12).TextColor(muted)
		})
	}
}

func (app *chatApp) providerChip(c *ui.Context, option modelOption) {
	selected := option.id == app.form.providerID
	chip := ui.Row(c).Height(33).MaxWidth(220).Shrink(0).Radius(17).AlignItems(ui.Center).
		Background(side)
	hovered := chip.Hovered()
	switch {
	case hovered && selected:
		chip.Background(ui.Hex("#D4E8DE"))
	case hovered:
		chip.Background(hover)
	case selected:
		chip.Background(tint)
	}
	chip.Children(func() {
		selectButton := pointerButton(c.Key("select"), false).Label(option.label).Tooltip(option.label).
			Height(33).MinWidth(0).MaxWidth(192).PaddingX(11).Radius(17).AlignItems(ui.Center).Gap(5).
			FocusRingColor(accent).Children(func() {
			if selected {
				icon(c, "check", accent, 12)
			}
			ui.Text(c, option.label).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13).FontWeight(620)
		}).OnClick(func() { app.editProvider(option.id) })
		if option.id == "" {
			return
		}
		expanded := hovered || selectButton.FocusVisible() || chip.FocusWithin() && !selectButton.Focused()
		var target float32
		if expanded {
			target = 1
		}
		progress := chip.Animate("remove-reveal", target, 160*time.Millisecond)
		if progress > 0 {
			label := locales.RemoveProvider(option.label)
			ui.Row(c.Key("remove-slot")).Width(28 * progress).Height(33).Shrink(0).
				AlignItems(ui.Center).Clip().Children(func() {
				button := pointerButton(c.Key("remove"), !expanded).Label(label).Tooltip(label).
					Size(24, 24).Shrink(0).Radius(12).Center().FocusRingColor(accent).Opacity(progress)
				color := muted
				if button.Hovered() {
					button.Background(side)
					color = errorFG
				}
				button.Children(func() { icon(c, "close", color, 13) }).OnClick(func() {
					app.form.providerToDelete, app.form.providerDeleteError = option.id, ""
				})
			})
		}
	})
}

func (app *chatApp) providerDeleteDialog(c *ui.Context) {
	form := &app.form
	provider := app.providers.provider(form.providerToDelete)
	if provider == nil {
		return
	}
	title := locales.DeleteProviderTitle()
	body := locales.DeleteProviderBody(provider.Name)
	ui.Overlay(c, func() {
		backdrop := ui.Box(c.Key("backdrop")).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Center().Background(ui.RGBA(0, 0, 0, .4)).Modal()
		cancel, confirm := backdrop.OverlayShortcut(0, ui.KeyEscape), false
		backdrop.Children(func() {
			ui.Column(c).Role(ui.RoleAlertDialog).Label(title).Description(body).
				Width(340).MaxWidthPercent(90).Padding(20).Gap(12).Radius(10).Background(white).
				Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, .3)).Children(func() {
				ui.Text(c, title).FontSize(16).FontWeight(680)
				ui.Text(c, body).FontSize(13).TextColor(muted)
				if form.providerDeleteError != "" {
					ui.Text(c, form.providerDeleteError).FontSize(12).TextColor(errorFG)
				}
				ui.Row(c).Wrap().Justify(ui.End).Gap(8).Children(func() {
					back := pointerButton(c.Key("cancel"), false).Label(locales.Cancel()).AutoFocus().
						Height(34).PaddingX(13).AlignItems(ui.Center).Radius(7).Background(side).Border(1, border)
					if back.Hovered() {
						back.Background(hover)
					}
					back.Children(func() { ui.Text(c, locales.Cancel()).FontSize(13) })
					remove := pointerButton(c.Key("confirm"), false).Label(locales.DeleteProvider()).
						Height(34).PaddingX(13).AlignItems(ui.Center).Radius(7).Background(errorFG)
					if remove.Hovered() {
						remove.Background(errorFG.Alpha(.88))
					}
					remove.Children(func() { ui.Text(c, locales.DeleteProvider()).FontSize(13).FontWeight(620).TextColor(white) })
					cancel = cancel || back.Clicked() || back.Shortcut(0, ui.KeyEnter)
					confirm = remove.Clicked() || remove.Shortcut(0, ui.KeyEnter)
				})
			})
		})
		if cancel {
			form.providerToDelete = ""
		} else if confirm {
			if err := app.deleteProvider(provider.ID); err != nil {
				form.providerDeleteError = err.Error()
			} else {
				form.providerToDelete = ""
			}
		}
	})
}

func (app *chatApp) connectionTestControl(c *ui.Context) {
	form := &app.form
	options := make([]modelOption, 0, len(form.models))
	for _, raw := range form.models {
		model := strings.TrimSpace(*raw)
		if model != "" && !slices.ContainsFunc(options, func(option modelOption) bool { return option.id == model }) {
			options = append(options, modelOption{id: model, label: model})
		}
	}
	selected := form.testModel
	if len(options) > 0 && !slices.ContainsFunc(options, func(option modelOption) bool { return option.id == selected }) {
		selected = form.defaultModelID()
		if selected == "" {
			selected = options[0].id
		}
	}
	if len(options) == 0 {
		selected = ""
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.TestModel()).FontSize(12).TextColor(muted)
		if selected != "" {
			modelSelect(c.Key("model-select"), locales.TestModel(), selected, &selected, options, 250, ui.Transparent, func() {
				form.testModel = selected
				app.clearConnectionTest()
			})
		}
	})
	var state *settings.ConnectionTestState
	if test := form.test; test != nil {
		state = &settings.ConnectionTestState{Running: test.cancel != nil, Failed: test.failed, Message: test.message}
	}
	settings.ConnectionTest(c.Key("test-button"), state, selected, func() { app.testConnection(selected) })
}
