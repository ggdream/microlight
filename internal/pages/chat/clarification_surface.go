package chat

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"

	"github.com/egoist/mygo/ui"
)

const clarificationAction = "answerClarification"

func clarificationMessage(id string, question clarification) (a2ui.AgentMessage, error) {
	var components []a2ui.Component
	var buildErr error
	add := func(id, kind string, props map[string]any) {
		if buildErr != nil {
			return
		}
		component, err := a2ui.NewComponent(id, kind, props)
		if err != nil {
			buildErr = err
			return
		}
		components = append(components, component)
	}
	addText := func(id, text string) {
		add(id, "Text", map[string]any{"text": text})
	}
	addButton := func(id, text string, value any, variant string) {
		addText(id+"_label", text)
		event := a2ui.Event{Name: clarificationAction, Context: map[string]any{"answer": value}}
		add(id, "Button", map[string]any{
			"child": id + "_label", "variant": variant,
			"action": map[string]any{"event": event},
		})
	}

	add("question", "Text", map[string]any{"text": question.displayTitle()})
	children := []string{"question"}
	var optionIDs []string
	for _, raw := range question.Options {
		option := clarificationOptionText(raw)
		if option == "" {
			continue
		}
		number := len(optionIDs) + 1
		id := fmt.Sprintf("option_%d", number)
		addButton(id, fmt.Sprintf("%d. %s", number, option), option, "default")
		optionIDs = append(optionIDs, id)
	}
	if len(optionIDs) > 0 {
		add("options", "Column", map[string]any{"children": optionIDs, "align": "start"})
		children = append(children, "options")
	}
	binding := a2ui.Binding{Path: "/answer"}
	add("answer", "TextField", map[string]any{
		"label": locales.YourAnswer(), "placeholder": locales.YourAnswerHint(), "value": binding,
	})
	addButton("submit", locales.Submit(), binding, "primary")
	add("controls", "Row", map[string]any{"children": []string{"answer", "submit"}})
	add(a2ui.Root, "Column", map[string]any{"children": append(children, "controls")})
	return a2ui.AgentMessage{Version: a2ui.Version, Create: &a2ui.CreateSurface{
		SurfaceRef: a2ui.SurfaceRef{ID: id}, Catalog: a2ui.BasicCatalog,
		Components: components, DataModel: map[string]any{"answer": ""},
	}}, buildErr
}

func (app *chatApp) clarificationForm(c *ui.Context, chat *conversation, reply *turn, question clarification) {
	id := question.CheckpointID
	surface := reply.clarificationSurface
	if surface == nil || surface.ID() != id {
		var processor a2ui.Processor
		message, err := clarificationMessage(id, question)
		if err == nil {
			err = processor.Apply(message)
		}
		if err != nil {
			slog.Error("build clarification form", "error", err)
			return
		}
		surface = processor.Surface(id)
		reply.clarificationSurface = surface
	}
	answerOf := func(message a2ui.RendererMessage) string {
		action := message.Action
		if action == nil || action.Name != clarificationAction || action.SurfaceID != id {
			return ""
		}
		answer, _ := action.Context["answer"].(string)
		return strings.TrimSpace(answer)
	}
	a2uimygo.View(c, surface, a2uimygo.Options{
		Style: a2uimygo.Style{
			Text: ink, Muted: muted, Border: border, Surface: white, Accent: accent, Tint: tint,
			Caret: caretColor, ControlHeight: replyControlHeight,
		},
		Disabled:     app.active != nil,
		CanActivate:  func(event a2ui.RendererMessage) bool { return answerOf(event) != "" },
		OnAction:     func(event a2ui.RendererMessage) { app.answerClarification(chat, reply, answerOf(event)) },
		EnterActions: map[string]string{"answer": "submit"},
		After: map[string]func(*ui.Context){"submit": func(c *ui.Context) {
			stopButton(c).OnClick(func() { app.stopReply(chat, reply) })
		}},
	})
}
