package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
)

type modelOption struct {
	id, label string
}

func modelSelect(c *ui.Context, label, value string, selected *string, options []modelOption, width float32, change func()) {
	selectModel := ui.SelectBase(c, selected)
	trigger := selectModel.Trigger.Label(label).Value(value).Tooltip(value).
		Height(windowLayout(c).Density(29, settings.SelectHeight(c))).MaxWidth(width).MinWidth(0).PaddingX(9).Radius(6).Gap(6).
		AlignItems(ui.Center).Background(side).Cursor(ui.CursorPointer)
	if trigger.Hovered() {
		trigger.Background(hover)
	}
	trigger.Children(func() {
		ui.Text(c, value).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 11)).FontWeight(620).TextColor(ink)
		ui.Icon(c, icons.SVG("ios_chevron_right")).Size(10, 10).Shrink(0).Rotate(90).TextColor(muted)
	}).OnChange(change)
	selectModel.Popup(func(panel ui.Element) {
		material3.Menu(panel.Width(windowLayout(c).FitWidth(240, 10)).MaxHeight(popoverHeight(c, trigger, 255)).Padding(4).Gap(2)).Radius(material3.ShapeMedium)
		rows := func() {
			for _, option := range options {
				item := selectModel.Item(option.id).Label(option.label).MinHeight(windowLayout(c).Target(28)).Padding(6, 8).
					AlignItems(ui.Center).Gap(8).Radius(5).Cursor(ui.CursorPointer)
				if item.Highlighted() || option.id == *selected {
					item.Background(hover)
				}
				item.Children(func() {
					ui.Text(c, option.label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 12))
					if option.id == *selected {
						icon(c, "check", accent, 13)
					}
				})
			}
		}
		if len(options) > 7 {
			scroll, offset := elastic.Scroll(c.Key("options"))
			scroll.FillWidth().MinHeight(0).Shrink(1).MaxHeight(230).Children(func() {
				offset.Apply(ui.Column(c)).FillWidth().Gap(2).Children(rows)
			})
		} else {
			rows()
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

func (app *chatApp) chatModelSelect(c *ui.Context) {
	current := app.currentModelChoice()
	provider := app.providers.provider(current.ProviderID)
	if provider == nil {
		return
	}
	value := provider.Name + " / " + current.Model
	layout := app.composerLayout(c)
	trigger := pointerButton(c.Key("choice"), false).Label(locales.ChooseChatModel()).Value(value).Tooltip(value).
		Height(composerControlHeight(layout, 32)).MaxWidth(layout.Density(172, 130)).MinWidth(0).Shrink(1).Radius(8).PaddingX(layout.Density(12, 7)).Gap(layout.Density(8, 5)).
		AlignItems(ui.Center).FocusRingColor(accent)
	open := ui.Local(trigger, "open", func() bool { return false })
	draftProvider := ui.Local(trigger, "provider", func() string { return current.ProviderID })
	background := hover
	switch {
	case trigger.Pressed(), trigger.FocusVisible():
		background = ink.Alpha(.12)
	case trigger.Hovered():
		background = ink.Alpha(.08)
	}
	material3.ButtonStyle(trigger, material3.TonalButton, false).Background(background).Expanded(*open).Children(func() {
		ui.Text(c, current.Model).MinWidth(0).SingleLine().Ellipsis("…").
			FontSize(viewport.Font(c, 12)).FontWeight(550).TextColor(accent)
		angle := float32(90)
		if *open {
			angle = -90
		}
		ui.Icon(c, icons.SVG("ios_chevron_right")).Size(10, 10).Shrink(0).Rotate(angle).TextColor(accent)
	}).OnClick(func() {
		if !*open {
			*draftProvider = current.ProviderID
		}
		*open = !*open
	})
	if layout.Compact() {
		if *open {
			selectionSheet(c.Key("model-sheet"), open, locales.ChooseChatModel(), .58, func() {
				ui.Column(c).Fill().Padding(layout.Y(12), layout.X(16)).Gap(layout.Y(12)).Children(func() {
					ui.Text(c, locales.ChooseChatModel()).FontSize(layout.Text(17)).FontWeight(620)
					ui.Row(c).Grow(1).MinHeight(0).Gap(layout.X(12)).Children(func() {
						scroll, offset := elastic.Scroll(c.Key("providers"))
						scroll.WidthPercent(34).FillHeight().Children(func() {
							offset.Apply(ui.Column(c)).FillWidth().Gap(4).Children(func() {
								for _, p := range app.providers.Providers {
									button := pointerButton(c.Key(p.ID), false).Label(p.Name).FillWidth().MinHeight(layout.Target(44)).Padding(8).Radius(8).FocusRingColor(accent)
									if p.ID == *draftProvider {
										button.Background(tint)
									} else if button.Hovered() {
										button.Background(hover)
									}
									button.Children(func() { ui.Text(c, p.Name).FontSize(layout.Text(14)).TextColor(ink) }).OnClick(func() { *draftProvider = p.ID })
								}
							})
						})
						material3.Divider(c, material3.DividerOptions{Vertical: true, Space: 1})
						ui.Column(c).Grow(1).FillHeight().MinHeight(0).MinWidth(0).Children(func() {
							picked := app.providers.provider(*draftProvider)
							if picked == nil {
								picked = provider
							}
							chatModelRows(c.Key(picked.ID), picked, current, func(name string) {
								choice := modelChoice{ProviderID: picked.ID}
								if name != picked.Model {
									choice.Model = name
								}
								app.selectProvider(choice)
								*open = false
							})
						})
					})
				})
			})
		}
		return
	}
	ui.PopoverBase(c.Key("picker"), trigger, open, func(panel ui.Element) {
		material3.Elevate(material3.Menu(panel.AttachTo(trigger, ui.AnchorTopEnd, ui.AnchorBottomEnd).Width(windowLayout(c).FitWidth(286, 8)).MaxHeight(popoverHeight(c, trigger, 268)).Margin(0, 0, 6).
			Padding(6).Gap(5)).Radius(material3.ShapeMedium), 2)

		app.chatModelProviderTabs(c.Key("providers"), draftProvider)
		ui.Box(c).Height(1).FillWidth().Margin(0, 2).Background(border)
		picked := app.providers.provider(*draftProvider)
		if picked == nil {
			*draftProvider = current.ProviderID
			picked = provider
		}
		chatModelRows(c.Key(picked.ID), picked, current, func(name string) {
			choice := modelChoice{ProviderID: picked.ID}
			if name != picked.Model {
				choice.Model = name
			}
			app.selectProvider(choice)
			*open = false
		})
	})
}

func (app *chatApp) chatModelProviderTabs(c *ui.Context, selected *string) {
	scroll, offset := elastic.Horizontal(c)
	scroll.FillWidth().Height(32).Children(func() {
		group := ui.RadioGroup(c.Key("providers"), func() {
			offset.Apply(ui.Row(c)).Gap(4).Padding(0, 2).AlignItems(ui.Center).Children(func() {
				for _, provider := range app.providers.Providers {
					tab := ui.RadioBase(c.Key(provider.ID), selected, provider.ID).
						Label(provider.Name).Tooltip(provider.Name).Height(28).MaxWidth(150).Shrink(0).
						PaddingX(9).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).FocusRingColor(accent)
					color := muted
					if provider.ID == *selected {
						color = accent
						tab.Background(tint)
					} else if tab.Hovered() {
						tab.Background(hover)
					}
					tab.Children(func() {
						ui.Text(c, provider.Name).MinWidth(0).SingleLine().Ellipsis("…").
							FontSize(12).FontWeight(580).TextColor(color)
					})
				}
			})
		})
		group.Label(locales.ModelProvider())
	})
}

func chatModelRows(c *ui.Context, provider *modelProvider, active modelChoice, choose func(string)) {
	rows := func() {
		for _, name := range provider.Models {
			selected := provider.ID == active.ProviderID && name == active.Model
			item := pointerButton(c.Key(name), false).Label(name).Tooltip(name).AccessSelected(selected).
				FillWidth().Height(windowLayout(c).Density(34, windowLayout(c).Target(34))).PaddingX(9).Gap(8).AlignItems(ui.Center).Radius(6).FocusRingColor(accent)
			if item.Hovered() {
				item.Background(hover)
			}
			item.Children(func() {
				ui.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13)).TextColor(ink)
				if selected {
					icon(c, "check", accent, 14)
				}
			}).OnClick(func() { choose(name) })
		}
	}
	scroll, offset := elastic.Scroll(c)
	scroll.Label(locales.ModelID()).FillWidth().Shrink(1).MinHeight(0)
	if windowLayout(c).Compact() {
		scroll.Grow(1)
	} else {
		scroll.MaxHeight(214)
	}
	scroll.Children(func() {
		offset.Apply(ui.Column(c)).FillWidth().Gap(2).Children(rows)
	})
}

func (app *chatApp) providerSettingsControl(c *ui.Context) {
	form := &app.form
	var options []modelOption
	for _, p := range orderedProviders(app.providers.Providers, form.providerOrder) {
		options = append(options, modelOption{id: p.ID, label: p.Name})
	}
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
			Shrink(0).Radius(material3.ShapeSmall).OnClick(func() { app.editProvider("") })
	})
}

func (app *chatApp) providerModelsControl(c *ui.Context) {
	form := &app.form
	settings.Card(c, locales.ProviderModels(), func() {
		ui.Text(c, locales.DefaultModelHint()).FontSize(viewport.Font(c, 12)).TextColor(muted)
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
							ring.Background(hover).Border(1, accent)
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
			ui.Text(c, label).FontSize(viewport.Font(c, 12)).FontWeight(610)
			modelSelect(c.Key("select"), label, *selected, selected, options, 180, change)
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
			ui.Text(c, locales.ModelApiRoute()).FontSize(viewport.Font(c, 12)).FontWeight(610)
			ui.Text(c, "Messages").FontSize(viewport.Font(c, 12)).TextColor(muted)
		})
	}
}

type providerChipDrag string

func (app *chatApp) providerChip(c *ui.Context, option modelOption) {
	selected := option.id == app.form.providerID
	chip := ui.Row(c).Height(33).MaxWidth(220).Shrink(0).Radius(material3.ShapeSmall).AlignItems(ui.Center).
		Background(side)
	hovered := chip.Hovered()
	material3.ChoiceChipStyle(chip, selected)
	foreground := material3.ChoiceChipForeground(selected)
	if option.id != "" {
		if source, dropped := ui.Drop[providerChipDrag](chip); dropped {
			app.moveProviderChip(string(source), option.id)
		}
		if source, over := ui.DragOver[providerChipDrag](chip); over && string(source) != option.id {
			chip.Draw(func(p *ui.Painter, box ui.Rect) { p.Stroke(box, foreground, material3.ShapeSmall, 2) })
		}
	}
	chip.Children(func() {
		selectButton := pointerButton(c.Key("select"), false).Label(option.label).Tooltip(option.label).
			AccessSelected(selected).
			Height(33).MinWidth(0).MaxWidth(192).PaddingX(11).Radius(material3.ShapeSmall).AlignItems(ui.Center).Gap(5).
			FocusRingColor(accent).Children(func() {
			if selected {
				icon(c, "check", foreground, 12)
			}
			ui.Text(c, option.label).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 13)).FontWeight(550).TextColor(foreground)
		}).OnClick(func() { app.editProvider(option.id) })
		if option.id == "" {
			return
		}
		selectButton.Drag(providerChipDrag(option.id))
		if selectButton.Dragging() {
			chip.Opacity(.55)
		}
		expanded := !selectButton.Dragging() && (hovered || selectButton.FocusVisible() || chip.FocusWithin() && !selectButton.Focused())
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
					Size(24, 24).Shrink(0).Radius(6).Center().FocusRingColor(accent).Opacity(progress)
				color := foreground.Alpha(.75)
				if button.Hovered() {
					button.Background(foreground.Alpha(.12))
					color = foreground
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
		insetDialog(c, backdrop)
		cancel, confirm := backdrop.OverlayShortcut(0, ui.KeyEscape), false
		backdrop.Children(func() {
			panel := material3.Elevate(ui.Column(c).Role(ui.RoleAlertDialog).Label(title).Description(body).
				Width(340).MaxWidthPercent(90).MaxHeight(dialogMaxHeight(c)).Padding(windowLayout(c).Density(20, 14)).Gap(windowLayout(c).Density(12, 8)).Radius(10).Background(white), 3)

			mobileDialog(c, backdrop, panel)
			panel.Children(func() {
				dialogBody(c, func() {
					ui.Text(c, title).FontSize(viewport.Font(c, 16)).FontWeight(680)
					ui.Text(c, body).FontSize(viewport.Font(c, 13)).TextColor(muted)
					if form.providerDeleteError != "" {
						ui.Text(c, form.providerDeleteError).FontSize(viewport.Font(c, 12)).TextColor(errorFG)
					}
				})
				dialogActions(c).Children(func() {
					back := material3.ButtonStyle(pointerButton(c.Key("cancel"), false).Label(locales.Cancel()).AutoFocus().
						Height(34).PaddingX(13).AlignItems(ui.Center), material3.TextButton, false)
					if back.Hovered() {
						back.Background(hover)
					}
					back.Children(func() { ui.Text(c, locales.Cancel()).FontSize(viewport.Font(c, 13)) })
					remove := pointerButton(c.Key("confirm"), false).Label(locales.DeleteProvider()).
						Height(34).PaddingX(13).AlignItems(ui.Center).Radius(material3.ShapeSmall).Background(errorFG)
					if remove.Hovered() {
						remove.Background(errorFG.Alpha(.88))
					}
					remove.Children(func() {
						ui.Text(c, locales.DeleteProvider()).FontSize(viewport.Font(c, 13)).FontWeight(620).TextColor(white)
					})
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
		ui.Text(c, locales.TestModel()).FontSize(viewport.Font(c, 12)).TextColor(muted)
		if selected != "" {
			modelSelect(c.Key("model-select"), locales.TestModel(), selected, &selected, options, 250, func() {
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
