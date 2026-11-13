package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
)

type interfaceUpdate struct {
	Offset   int                 `json:"offset"`
	Messages []a2ui.AgentMessage `json:"messages,omitempty"`
	Invalid  bool                `json:"invalid,omitempty"`
	Local    bool                `json:"local,omitempty"`
}

type storedInterface struct {
	Updates []interfaceUpdate     `json:"updates,omitempty"`
	Action  *a2ui.RendererMessage `json:"action,omitempty"`
}

type interfaceAnchor struct {
	owner  *turn
	offset int
	id     string
	key    int
}

type interfaceProjection struct {
	state   *a2ui.Processor
	anchors map[string]interfaceAnchor
	rows    map[*turn][]interfaceAnchor
}

func projectInterface(turns []*turn) *interfaceProjection {
	view := &interfaceProjection{
		state: new(a2ui.Processor), anchors: make(map[string]interfaceAnchor),
		rows: make(map[*turn][]interfaceAnchor),
	}
	for _, entry := range turns {
		for i, update := range entry.Interface {
			view.apply(entry, i, update)
		}
	}
	return view
}

func (view *interfaceProjection) apply(entry *turn, index int, update interfaceUpdate) {
	if len(update.Messages) == 0 && !update.Invalid {
		return
	}
	if update.Invalid || view.state.ApplyBatch(update.Messages) != nil {
		view.rows[entry] = append(view.rows[entry], interfaceAnchor{owner: entry, offset: update.Offset, key: index})
		return
	}
	for _, msg := range update.Messages {
		switch {
		case msg.Create != nil:
			id := msg.Create.ID
			anchor := interfaceAnchor{entry, update.Offset, id, index}
			view.anchors[id] = anchor
			view.rows[entry] = append(view.rows[entry], anchor)
		case msg.Delete != nil:
			delete(view.anchors, msg.Delete.ID)
		}
	}
}

func (view *interfaceProjection) activeRows(entry *turn) []interfaceAnchor {
	return slices.DeleteFunc(slices.Clone(view.rows[entry]), func(row interfaceAnchor) bool {
		return row.id != "" && view.anchors[row.id] != row
	})
}

type interfaceContextKey struct{}

type interfaceRequest struct {
	state *a2ui.Processor
	ids   []string
}

var interfaceCatalogs = sync.OnceValues(func() ([]*a2ui.Catalog, error) {
	return (a2uiagent.Config{}).Negotiate(nil)
})

func (request *interfaceRequest) stream() (*a2uiagent.Stream, error) {
	catalogs, err := interfaceCatalogs()
	if err != nil {
		return nil, err
	}
	return (a2uiagent.Config{Catalogs: catalogs, ContinueAfterError: true}).NewStream(
		a2uiagent.Mixed, request.state.Capabilities(false), request.state.Fork())
}

type interfaceMiddleware struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
}

func (*interfaceMiddleware) ChatModelAgentMiddlewareName() string { return "microlight-a2ui" }

func (*interfaceMiddleware) WrapModel(_ context.Context, next model.BaseModel[*schema.AgenticMessage], _ *adk.TypedModelContext[*schema.AgenticMessage]) (model.BaseModel[*schema.AgenticMessage], error) {
	return interfaceModel{next}, nil
}

type interfaceModel struct {
	model.BaseModel[*schema.AgenticMessage]
}

func (m interfaceModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	return m.BaseModel.Generate(ctx, interfaceGuidance(ctx, input), opts...)
}

func (m interfaceModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return m.BaseModel.Stream(ctx, interfaceGuidance(ctx, input), opts...)
}

func interfaceGuidance(ctx context.Context, input []*schema.AgenticMessage) []*schema.AgenticMessage {
	request, _ := ctx.Value(interfaceContextKey{}).(*interfaceRequest)
	if request == nil {
		return input
	}
	catalogs, err := interfaceCatalogs()
	if err != nil {
		return input
	}
	guidance, err := a2uiagent.PromptFor(a2uiagent.Mixed, catalogs, request.state.DataModel())
	if err != nil {
		return input
	}
	guidance += "\nUse a surface for forms, interactive choices, and media playback when it helps. For ordinary replies, use Markdown. Use ask_user when a tool run must pause for clarification, and always use the normal approval flow for tool permissions. Treat renderer events as structured user input and only open links in response to user activation. A native video needs an HTTP(S) URL for a direct media file, such as an MP4, rather than a page URL. Use a unique surface ID when creating a new interface.\n"
	if len(request.ids) != 0 {
		ids, _ := json.Marshal(request.ids)
		guidance += "\nActive surface IDs in this branch: " + string(ids)
	}
	if len(input) > 0 && input[0].Role == schema.AgenticRoleTypeSystem {
		out := slices.Clone(input)
		system := *out[0]
		system.ContentBlocks = append(slices.Clone(system.ContentBlocks), schema.NewContentBlock(&schema.UserInputText{Text: guidance}))
		out[0] = &system
		return out
	}
	return append([]*schema.AgenticMessage{schema.SystemAgenticMessage(guidance)}, input...)
}

func (app *chatApp) sendInterfaceAction(chat *conversation, event a2ui.RendererMessage) {
	if app.selected() != chat || app.active != nil || chat.waitingReply() != nil || app.backend == nil || app.backend.configErr != nil || event.Action == nil {
		return
	}
	view := chat.interfaceView()
	if surface := view.state.Surface(event.Action.SurfaceID); surface == nil {
		return
	} else if _, ok := surface.Component(event.Action.SourceComponentID); !ok {
		return
	}
	label := strings.TrimSpace(event.Action.UserMessage)
	if label == "" {
		label = event.Action.Name
	}
	chat.Turns = append(chat.Turns, &turn{
		message: message{Role: "user", Content: label}, InterfaceAction: &event,
		StartedAt: time.Now(),
	})
	app.generate(chat, len(chat.Turns))
}

func (chat *conversation) interfaceView() *interfaceProjection {
	if chat.projectedInterface == nil {
		chat.projectedInterface = projectInterface(chat.Turns)
	}
	return chat.projectedInterface
}

func (chat *conversation) interfaceRequest() *interfaceRequest {
	view := chat.interfaceView()
	return &interfaceRequest{view.state.Fork(), slices.Sorted(maps.Keys(view.anchors))}
}

func (chat *conversation) saveInterfaceData(entry *turn, id, path string, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	if last := len(entry.Interface) - 1; last >= 0 {
		previous := entry.Interface[last]
		if previous.Local && len(previous.Messages) == 1 {
			if data := previous.Messages[0].Data; data != nil && data.ID == id && data.Path == path {
				next := *data
				next.Value = raw
				entry.Interface[last].Messages = []a2ui.AgentMessage{{Version: a2ui.Version, Data: &next}}
				return
			}
		}
	}
	entry.Interface = append(entry.Interface, interfaceUpdate{
		Offset: len(entry.Content), Local: true,
		Messages: []a2ui.AgentMessage{{
			Version: a2ui.Version, Data: &a2ui.UpdateDataModel{
				SurfaceRef: a2ui.SurfaceRef{ID: id}, Path: path, Value: raw,
			},
		}},
	})
}

func interfaceEventInput(event a2ui.RendererMessage) (*schema.AgenticMessage, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	if _, err := a2ui.DecodeRenderer(raw); err != nil {
		return nil, err
	}
	return schema.UserAgenticMessage(fmt.Sprintf("A2UI renderer event (JSON):\n%s", raw)), nil
}

func (request *interfaceRequest) advance(state *a2ui.Processor, updates []interfaceUpdate) *interfaceRequest {
	ids := make(map[string]bool, len(request.ids))
	for _, id := range request.ids {
		ids[id] = true
	}
	for _, update := range updates {
		for _, message := range update.Messages {
			if message.Create != nil {
				ids[message.Create.ID] = true
			}
			if message.Delete != nil {
				delete(ids, message.Delete.ID)
			}
		}
	}
	return &interfaceRequest{state, slices.Sorted(maps.Keys(ids))}
}

func (chat *conversation) commitInterface(entry *turn, offset int, updates []interfaceUpdate) {
	view := chat.interfaceView()
	for _, update := range updates {
		update.Offset += offset
		view.apply(entry, len(entry.Interface), update)
		entry.Interface = append(entry.Interface, update)
	}
}
