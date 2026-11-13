package chat

import (
	"net/http"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
)

func (chat *conversation) transcriptInterface() *interfaceProjection {
	view := chat.interfaceView()
	if len(chat.Turns) == 0 {
		return view
	}
	entry := chat.Turns[len(chat.Turns)-1]
	if len(entry.interfacePreview) == 0 {
		return view
	}
	draft := &interfaceProjection{
		state: view.state.Fork(), anchors: make(map[string]interfaceAnchor, len(view.anchors)),
		rows: make(map[*turn][]interfaceAnchor, len(view.rows)),
	}
	for id, anchor := range view.anchors {
		draft.anchors[id] = anchor
	}
	for owner, rows := range view.rows {
		draft.rows[owner] = rows
	}
	draft.rows[entry] = append([]interfaceAnchor(nil), view.rows[entry]...)
	for i, update := range entry.interfacePreview {
		draft.apply(entry, len(entry.Interface)+i, update)
	}
	return draft
}

func (app *chatApp) renderInterfaceAnswer(c *ui.Context, chat *conversation, entry *turn, view *interfaceProjection, rows []interfaceAnchor) {
	content := entry.Content
	if len(entry.interfacePreview) != 0 {
		content += entry.processPreview
	}
	start := 0
	for i, row := range rows {
		end := min(max(row.offset, start), len(content))
		if text := content[start:end]; strings.TrimSpace(text) != "" {
			app.replyText(c.Key(i), text, entry.State == "pending")
		}
		start = end
		if row.id == "" {
			interfaceFallback(c.Key(row), nil)
			continue
		}
		surface := view.state.Surface(row.id)
		enabled := app.active == nil && chat.waitingReply() == nil && app.backend != nil && app.backend.configErr == nil
		var client *http.Client
		if app.backend != nil && app.backend.network != nil {
			client = &http.Client{
				Transport: app.backend.network,
				CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
		}
		a2uimygo.View(c.Key(row), surface, a2uimygo.Options{
			Style:    a2uimygo.Style{Text: ink, Muted: muted, Border: border, Surface: white, Accent: accent, Tint: tint, Caret: caretColor},
			Disabled: !enabled,
			Fallback: interfaceFallback,
			Media: a2uimygo.MediaOptions{
				Client: client, RequireActivation: true, LoadLabel: locales.A2uiLoadMedia(),
				UnavailableLabel: locales.A2uiMediaUnavailable(),
			},
			OnAction: func(event a2ui.RendererMessage) { app.sendInterfaceAction(chat, event) },
			OnDataChange: func(path string, value any) {
				if !enabled || len(chat.Turns) == 0 || chat.interfaceView().state.Surface(row.id) != surface {
					return
				}
				chat.saveInterfaceData(chat.Turns[len(chat.Turns)-1], row.id, path, value)
				app.save(chat)
			},
		})
	}
	if text := content[start:]; strings.TrimSpace(text) != "" {
		app.replyText(c.Key("answer-end"), text, entry.State == "pending")
	}
}

func interfaceFallback(c *ui.Context, _ error) {
	ui.Text(c, locales.A2uiRenderFailed()).FontSize(12).TextColor(muted)
}
