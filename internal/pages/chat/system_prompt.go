package chat

import (
	"context"
	"github.com/ggdream/microlight/pkg/viewport"
	"log/slog"
	"slices"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/internal/theme"
)

const systemPromptConfigKey = "system_prompt"

func (app *chatApp) loadSystemPrompt() {
	if app.store == nil {
		return
	}
	prompt, err := app.store.getConfig(systemPromptConfigKey)
	if err != nil {
		slog.Error("read system prompt preference", "error", err)
		return
	}
	app.systemPrompt = prompt
}

func (app *chatApp) saveSystemPrompt(prompt string) {
	if app.deferBrowserAction(func() { app.saveSystemPrompt(prompt) }) {
		app.systemPrompt = prompt
		return
	}
	if app.store == nil || storage.PutConfig(app.store.DB, systemPromptConfigKey, prompt) != nil {
		app.form.generalStatus = settingsStatus{message: locales.SettingsSaveFailed(), failed: true}
		return
	}
	if !desktop.Web {
		app.systemPrompt = prompt
	}
	app.form.generalStatus = settingsStatus{}
}

func (app *chatApp) systemPromptSettings(c *ui.Context) {
	label := locales.SystemPrompt()
	layout := windowLayout(c)
	settings.Card(c, label, func() {
		value := app.systemPrompt
		ui.Text(c, locales.SystemPromptDescription()).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
		theme.InputFrame(ui.TextAreaBase(c.Key("content"), &value), 12, 7, theme.Border).
			Label(label).Placeholder(locales.SystemPromptPlaceholder()).FillWidth().Height(layout.Y(108)).PaddingY(layout.Y(10)).
			Background(theme.Canvas).FontSize(viewport.Font(c, 13)).CaretColor(theme.CaretColor).
			OnChange(func() { app.saveSystemPrompt(value) })
	})
}

type systemPromptKey struct{}

type systemPromptMiddleware struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
}

func (*systemPromptMiddleware) ChatModelAgentMiddlewareName() string {
	return "microlight-system-prompt"
}

func (*systemPromptMiddleware) WrapModel(_ context.Context, next model.BaseModel[*schema.AgenticMessage], _ *adk.TypedModelContext[*schema.AgenticMessage]) (model.BaseModel[*schema.AgenticMessage], error) {
	return systemPromptModel{next}, nil
}

type systemPromptModel struct {
	model.BaseModel[*schema.AgenticMessage]
}

func (m systemPromptModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	return m.BaseModel.Generate(ctx, addSystemPrompt(ctx, input), opts...)
}

func (m systemPromptModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return m.BaseModel.Stream(ctx, addSystemPrompt(ctx, input), opts...)
}

// Keep the prompt in the model request so changes also apply to existing sessions.
func addSystemPrompt(ctx context.Context, input []*schema.AgenticMessage) []*schema.AgenticMessage {
	prompt, _ := ctx.Value(systemPromptKey{}).(string)
	if prompt == "" {
		return input
	}
	text := "User-configured standing instructions. Apply them alongside the built-in guidance. Honor the latest user request for task-specific preferences and keep all tool access and approval rules in effect.\n\n" + prompt
	if len(input) == 0 || input[0].Role != schema.AgenticRoleTypeSystem {
		return append([]*schema.AgenticMessage{schema.SystemAgenticMessage(text)}, input...)
	}
	out := slices.Clone(input)
	system := *out[0]
	system.ContentBlocks = append(slices.Clone(system.ContentBlocks), schema.NewContentBlock(&schema.UserInputText{Text: text}))
	out[0] = &system
	return out
}
