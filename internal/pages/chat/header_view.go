package chat

import (
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

const compactHistoryIcon = "navigation"

func (app *chatApp) topbar(c *ui.Context, selected *conversation) {
	layout := windowLayout(c)
	height := layout.Y(57)
	left, right, gap := layout.Gutter(25), layout.Gutter(25), layout.Gutter(14)
	if layout.Compact() {
		height = max(height, layout.Target(44)+2)
		// Balance the visible menu glyph against the title's trailing edge,
		// while keeping the menu's full touch target.
		right += (layout.Target(44) - 20) / 2
		gap = layout.X(4)
	}
	ui.Row(c).Height(height).Shrink(0).Padding(0, right, 0, left).AlignItems(ui.Center).Gap(gap).
		BorderWidth(0, 0, 1, 0).BorderColor(border).Children(func() {
		if layout.Compact() {
			app.compactNavButton(c.Key("history"), locales.ChatHistory(), compactHistoryIcon, func() { app.compactHistory = true })
		}
		title := locales.UntitledChat()
		if selected != nil {
			title = selected.Title
		}
		ui.Text(c, title).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Clip().
			FontSize(layout.Text(13)).FontWeight(640)
		if app.overlappingWorkspace(selected) {
			ui.Box(c.Key("concurrent-workspace")).Tooltip(locales.WorkspaceConcurrent()).Label(locales.WorkspaceConcurrent()).Shrink(0).Children(func() {
				icon(c, "shield_error", errorFG, 16)
			})
		}
		if selected != nil && len(selected.Turns) > 0 && !selected.list.AtEnd() {
			quietButton(c, locales.BackToLatest(), "down").OnClick(selected.list.ScrollToEnd)
		}
		if selected != nil && len(selected.Turns) > 0 {
			round := selected.Turns[len(selected.Turns)-1]
			if len(roundChangedFiles(round, selected.workspace)) > 0 {
				messageIcon(c.Key("round-changes"), locales.RoundChanges(), "document_sync", false, func() {
					app.openRoundChanges(round)
				})
			}
		}
		app.backgroundTaskIndicator(c.Key("background-tasks"))
	})
}

func (app *chatApp) welcome(c *ui.Context) {
	layout := windowLayout(c)
	if layout.Compact() || layout.Height < 540 {
		scroll, offset := elastic.Scroll(c)
		height := scroll.LayoutBounds().H
		scroll.FillWidth().Grow(1).MinHeight(0).Children(func() {
			offset.Apply(ui.Column(c)).FillWidth().MinHeight(height).Justify(ui.Center).
				Children(func() { app.welcomeContent(c) })
		})
		if height == 0 {
			c.AnimationFrame()
		}
		return
	}
	app.welcomeContent(c)
}

func (app *chatApp) welcomeContent(c *ui.Context) {
	layout := windowLayout(c)
	pair := func(c *ui.Context) ui.Element {
		return ui.Row(c).FillWidth().Gap(layout.X(layout.Density(10, 7)))
	}
	ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).AlignItems(ui.Center).Padding(layout.Gutter(20)).Children(func() {
		logoSize := layout.X(layout.Density(68, 64))
		ui.Image(c, app.brandIcon).Size(logoSize, logoSize).Shrink(0).Fit(ui.Contain)
		ui.Text(c, locales.AppName()).FontSize(layout.Text(layout.Density(36, 34))).FontWeight(680).
			Margin(layout.Y(layout.Density(17, 12)), 0, layout.Y(6))
		ui.Text(c, locales.AppSubtitle()).FontSize(viewport.Font(c, 13)).TextColor(muted)
		ui.Column(c).Gap(layout.Y(layout.Density(10, 7))).FillWidth().MaxWidth(510).
			Margin(layout.Y(layout.Density(22, 14)), 0, 0).Children(func() {
			pair(c.Key("daily-suggestions")).Children(func() {
				app.suggestion(c.Key("weight"), locales.WeightSuggestion(), locales.WeightSuggestionHint(), "heart_pulse", locales.WeightSuggestionPrompt())
				app.suggestion(c.Key("outfit"), locales.OutfitSuggestion(), locales.OutfitSuggestionHint(), "shirt", locales.OutfitSuggestionPrompt())
			})
			pair(c.Key("travel-suggestions")).Children(func() {
				app.suggestion(c.Key("travel"), locales.TravelSuggestion(), locales.TravelSuggestionHint(), "globe", locales.TravelSuggestionPrompt())
				app.suggestion(c.Key("route"), locales.RouteSuggestion(), locales.RouteSuggestionHint(), "route", locales.RouteSuggestionPrompt())
			})
		})
	})
}

func (app *chatApp) suggestion(c *ui.Context, title, detail, glyph, prompt string) {
	layout := windowLayout(c)
	button := pointerButton(c, false).Label(title).Grow(1).Basis(0).MinWidth(0).
		MinHeight(layout.Target(layout.Density(60, 54))).PaddingX(layout.X(layout.Density(12, 7))).
		PaddingY(layout.Y(layout.Density(6, 4))).Gap(layout.X(layout.Density(10, 6))).
		AlignItems(ui.Center).Radius(10).Background(white).Border(1, border)
	if button.Hovered() {
		button.Background(side)
	}
	button.Children(func() {
		size := layout.X(layout.Density(30, 25))
		ui.Column(c).Size(size, size).Shrink(0).Radius(7).Background(tint).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() { icon(c, glyph, accent, 16) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(layout.Y(3)).Children(func() {
			ui.Text(c, title).FontSize(viewport.Font(c, 13)).FontWeight(640).SingleLine().Ellipsis("…")
			ui.Text(c, detail).FontSize(viewport.Font(c, 11)).TextColor(muted).MaxLines(2).Ellipsis("…")
		})
		if !layout.Compact() {
			icon(c, "open", muted, 12)
		}
	}).OnClick(func() {
		app.draft.SetText(prompt)
		app.focusComposer = true
	})
}

func (app *chatApp) compactNavButton(c *ui.Context, label, glyph string, click func()) {
	size := windowLayout(c).Target(44)
	pointerButton(c, false).Label(label).Size(size, size).Shrink(0).Radius(8).Center().
		Children(func() { icon(c, glyph, muted, 20) }).OnClick(click)
}
