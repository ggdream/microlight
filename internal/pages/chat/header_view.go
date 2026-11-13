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
		if selected != nil && len(selected.Turns) > 0 && !selected.list.AtEnd() {
			quietButton(c, locales.BackToLatest(), "down").OnClick(selected.list.ScrollToEnd)
		}
		if selected != nil && len(selected.Turns) > 0 {
			round := selected.Turns[len(selected.Turns)-1]
			if len(roundChangedFiles(round, selected.workspace)) > 0 {
				messageIcon(c.Key("round-changes"), locales.RoundChanges(), "document_sync", false, func() {
					app.toolPanel.visible, app.toolPanel.round = true, round
					app.toolPanel.active, app.toolPanel.reveal = nil, nil
				})
			}
		}
		app.chatProviderSelect(c.Key("provider"))
	})
}

func (app *chatApp) welcome(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).AlignItems(ui.Center).Padding(20).Children(func() {
		ui.Box(c).Width(48).Height(48).Radius(14).Background(tint).Draw(drawSpark(accent))
		ui.Text(c, locales.Welcome()).FontSize(34).FontWeight(680).Margin(17, 0, 6)
		ui.Text(c, locales.WelcomeHint()).FontSize(13).TextColor(muted)
		ui.Row(c).Gap(10).Justify(ui.Center).Wrap().FillWidth().MaxWidth(590).Margin(27, 0, 0).Children(func() {
			app.suggestion(c, locales.CodeSuggestion(), locales.CodeSuggestionHint(), "code",
				locales.CodeSuggestionPrompt())
			app.suggestion(c, locales.OutlineSuggestion(), locales.OutlineSuggestionHint(), "outline",
				locales.OutlineSuggestionPrompt())
		})
	})
}

func (app *chatApp) suggestion(c *ui.Context, title, detail, glyph, prompt string) {
	button := pointerButton(c, false).Label(title).Width(250).Height(60).PaddingX(12).
		Gap(10).AlignItems(ui.Center).Radius(10).Background(white).Border(1, border)
	if button.Hovered() {
		button.Background(side)
	}
	button.Children(func() {
		ui.Column(c).Width(30).Height(30).Radius(7).Background(tint).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() { icon(c, glyph, accent, 16) })
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(640)
			ui.Text(c, detail).FontSize(11).TextColor(muted)
		})
		icon(c, "open", muted, 12)
	}).OnClick(func() { app.draft.SetText(prompt) })
}
