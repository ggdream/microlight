package chat

import (
	"slices"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
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
	settings.Card(c, locales.ModelProvider(), func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			id := form.providerID
			options := app.providerOptions()
			label := locales.NewProvider()
			if p := app.providers.provider(id); p != nil {
				label = p.Name
			}
			if _, drafting := form.providerDrafts[""]; drafting || id == "" {
				options = append(options, modelOption{label: locales.NewProvider()})
			}
			modelSelect(c.Key("provider-edit"), locales.ModelProvider(), label, &id, options, 260, ui.Transparent, func() {
				app.editProvider(id)
			})
			settings.ActionButton(c.Key("new-provider"), icons, locales.AddProvider(), "plus", id == "").
				OnClick(func() { app.editProvider("") })
		})
		settings.Field(c, locales.ProviderName(), "DeepSeek", &form.providerName)
	})
}

func (app *chatApp) providerModelsControl(c *ui.Context) {
	form := &app.form
	settings.Card(c, locales.ProviderModels(), func() {
		ui.Text(c, locales.DefaultModel()).FontSize(12).FontWeight(610).TextColor(muted)
		settings.Field(c.Key("default-model"), locales.ModelID(), locales.ModelIDHint(), &form.model).
			OnChange(app.clearConnectionTest)
		for i, model := range form.extraModels {
			ui.Row(c.Key(model)).FillWidth().Gap(6).AlignItems(ui.End).Children(func() {
				settings.Field(c, locales.ModelID(), locales.ModelIDHint(), model)
				settings.ActionButton(c.Key("make-default"), icons, locales.MakeDefaultModel(), "check", *model == "").
					OnClick(func() {
						form.model, *model = *model, form.model
						app.clearConnectionTest()
					})
				messageIcon(c.Key("remove-model"), locales.RemoveProviderModel(), "close", false, func() {
					form.extraModels = slices.Delete(form.extraModels, i, i+1)
				})
			})
		}
		settings.ActionButton(c.Key("add-model"), icons, locales.AddProviderModel(), "plus", false).OnClick(func() {
			form.extraModels = append(form.extraModels, new(string))
		})
	})
}
