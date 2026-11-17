package chat

import (
	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func (app *chatApp) topbar(c *ui.Context, selected *conversation) {
	ui.Row(c).Height(57).Shrink(0).PaddingX(25).AlignItems(ui.Center).Gap(14).
		BorderWidth(0, 0, 1, 0).BorderColor(border).Children(func() {
		title := locales.UntitledChat()
		if selected != nil {
			title = selected.Title
		}
		ui.Text(c, title).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Clip().
			FontSize(13).FontWeight(640)
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
	ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).AlignItems(ui.Center).Padding(20).Children(func() {
		ui.Image(c, app.brandIcon).Size(58, 58).Shrink(0).Fit(ui.Contain)
		ui.Text(c, locales.AppName()).FontSize(34).FontWeight(680).Margin(17, 0, 6)
		ui.Text(c, locales.AppSubtitle()).FontSize(13).TextColor(muted)
		ui.Column(c).Gap(10).FillWidth().MaxWidth(510).Margin(22, 0, 0).Children(func() {
			ui.Row(c.Key("daily-suggestions")).FillWidth().Gap(10).Children(func() {
				app.suggestion(c.Key("weight"), locales.WeightSuggestion(), locales.WeightSuggestionHint(), "heart_pulse", locales.WeightSuggestionPrompt())
				app.suggestion(c.Key("outfit"), locales.OutfitSuggestion(), locales.OutfitSuggestionHint(), "shirt", locales.OutfitSuggestionPrompt())
			})
			ui.Row(c.Key("travel-suggestions")).FillWidth().Gap(10).Children(func() {
				app.suggestion(c.Key("travel"), locales.TravelSuggestion(), locales.TravelSuggestionHint(), "globe", locales.TravelSuggestionPrompt())
				app.suggestion(c.Key("route"), locales.RouteSuggestion(), locales.RouteSuggestionHint(), "route", locales.RouteSuggestionPrompt())
			})
		})
	})
}

func (app *chatApp) suggestion(c *ui.Context, title, detail, glyph, prompt string) {
	button := pointerButton(c, false).Label(title).Grow(1).Basis(0).MinWidth(0).Height(60).PaddingX(12).
		Gap(10).AlignItems(ui.Center).Radius(10).Background(white).Border(1, border)
	if button.Hovered() {
		button.Background(side)
	}
	button.Children(func() {
		ui.Column(c).Size(30, 30).Shrink(0).Radius(7).Background(tint).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() { icon(c, glyph, accent, 16) })
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(640).SingleLine().Ellipsis("…")
			ui.Text(c, detail).FontSize(11).TextColor(muted).SingleLine().Ellipsis("…")
		})
		icon(c, "open", muted, 12)
	}).OnClick(func() {
		app.draft.SetText(prompt)
		app.focusComposer = true
	})
}
