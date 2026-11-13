package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/proxy"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultModel   = "deepseek-flash"
	defaultBaseURL = "https://api.deepseek.com"
)

type message struct {
	Role        string             `json:"role"`
	Content     string             `json:"content"`
	Attachments []*attachment.File `json:"-"`
}

type Chat struct {
	network            *proxy.Transport
	skillsHome         string
	model              *openai.ChatModel
	configErr          error
	agent              adk.Agent
	buildAgent         func(context.Context, string) (adk.Agent, error)
	buildExtendedAgent func(context.Context, string, *agentExtensions) (adk.Agent, error)
	config             modelSettings
}

type replyDelta struct {
	text         string
	reasoning    string
	compression  compressionUpdate
	preview      string
	messageDone  bool
	activity     *[]timeline.Entry
	toolResult   *localtools.Result
	toolFinished string
}

type compressionUpdate uint8

const (
	compressionStarted compressionUpdate = iota + 1
	compressionFinished
)

func (chat *Chat) stream(ctx context.Context, messages []message, emit func(replyDelta) error) error {
	if chat.configErr != nil {
		return chat.configErr
	}
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return errors.New(locales.EnterMessage())
	}
	history, err := modelMessages(messages)
	if err != nil {
		return err
	}
	return chat.readReply(ctx, history, emit)
}

func modelMessages(messages []message) ([]*schema.Message, error) {
	var history []*schema.Message
	var imageBytes int
	for _, m := range messages {
		switch m.Role {
		case "user":
			if strings.TrimSpace(m.Content) == "" && len(m.Attachments) == 0 {
				return nil, errors.New(locales.EmptyMessage())
			}
			for _, attachment := range m.Attachments {
				if attachment.IsImage() {
					imageBytes += len(attachment.Data)
				}
			}
			if imageBytes > attachment.MaxRequestImageBytes {
				return nil, errors.New(locales.HistoryImagesTooLarge())
			}
			history = append(history, m.userMessage())
		case "assistant":
			history = append(history, schema.AssistantMessage(m.Content, nil))
		default:
			return nil, errors.New(locales.UnsupportedRole())
		}
	}
	return history, nil
}

func (chat *Chat) readReply(ctx context.Context, history []*schema.Message, emit func(replyDelta) error) error {
	stream, err := chat.model.Stream(ctx, history)
	if err != nil {
		return streamError(ctx, err)
	}
	defer stream.Close()
	hasText := false
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			if ctx.Err() != nil {
				return streamError(ctx, ctx.Err())
			}
			if hasText {
				return nil
			}
			return errors.New(locales.EmptyReply())
		}
		if err != nil {
			return streamError(ctx, err)
		}
		if chunk == nil || (chunk.Content == "" && chunk.ReasoningContent == "") {
			continue
		}
		hasText = hasText || strings.TrimSpace(chunk.Content) != ""
		if err := emit(replyDelta{text: chunk.Content, reasoning: chunk.ReasoningContent}); err != nil {
			return streamError(ctx, err)
		}
	}
}

func (m message) userMessage() *schema.Message {
	if len(m.Attachments) == 0 {
		return schema.UserMessage(m.Content)
	}
	text := m.Content
	var images []schema.MessageInputPart
	var files strings.Builder
	const maxTextAttachmentBytes = 32 << 10
	remaining := maxTextAttachmentBytes
	for _, attachment := range m.Attachments {
		if !attachment.IsImage() {
			file := struct {
				Name      string  `json:"name"`
				Text      *string `json:"text,omitempty"`
				Truncated bool    `json:"truncated,omitempty"`
			}{Name: attachment.Name}
			if attachment.IsText() {
				end := min(remaining, len(attachment.Data))
				for end < len(attachment.Data) && end > 0 && !utf8.RuneStart(attachment.Data[end]) {
					end--
				}
				content := string(attachment.Data[:end])
				file.Text, file.Truncated = &content, end < len(attachment.Data)
				remaining -= end
			}
			data, _ := json.Marshal(file)
			files.WriteByte('\n')
			files.Write(data)
			continue
		}
		data := base64.StdEncoding.EncodeToString(attachment.Data)
		images = append(images, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{
				MIMEType:   attachment.MIME,
				Base64Data: &data,
			}},
		})
	}
	if strings.TrimSpace(text) == "" {
		text = locales.DescribeImages()
		if files.Len() != 0 {
			text = locales.ReviewAttachments()
		}
	}
	if files.Len() != 0 {
		text += "\n\nUser attachments (JSON lines, supplied as reference data; missing text means the file contents are unavailable):" + files.String()
	}
	if len(images) == 0 {
		return schema.UserMessage(text)
	}
	parts := []schema.MessageInputPart{{Type: schema.ChatMessagePartTypeText, Text: text}}
	return &schema.Message{Role: schema.User, UserInputMultiContent: append(parts, images...)}
}

func streamError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return errors.New(locales.RequestTimedOut())
	}
	if errors.Is(err, proxy.ErrProxy) {
		return errors.New(locales.ProxyRequestFailed())
	}
	return errors.New(locales.ConnectionFailed())
}

func newChat(config modelSettings) *Chat {
	endpoint, err := url.Parse(config.BaseURL)
	if err != nil {
		return &Chat{configErr: errors.New(locales.ReadBaseURLFailed())}
	}
	network, err := proxy.NewTransport(config.Proxy)
	if err != nil {
		return &Chat{configErr: errors.New(locales.InvalidProxy())}
	}
	opts := &openai.ChatModelConfig{
		APIKey:      config.APIKey,
		BaseURL:     config.BaseURL,
		Model:       config.Model,
		Temperature: config.Temperature,
		TopP:        config.TopP,
		HTTPClient:  &http.Client{Transport: network, Timeout: 2 * time.Minute},
	}
	if strings.EqualFold(endpoint.Hostname(), "api.deepseek.com") {
		opts.ExtraFields = map[string]any{"thinking": map[string]string{"type": "enabled"}}
	}
	if config.TopP != nil && *config.TopP == 0 {
		if opts.ExtraFields == nil {
			opts.ExtraFields = make(map[string]any)
		}
		opts.ExtraFields["top_p"] = 0
	}
	if strings.EqualFold(endpoint.Hostname(), "api.openai.com") {
		opts.MaxCompletionTokens = config.MaxTokens
	} else {
		opts.MaxTokens = config.MaxTokens
	}
	client, err := openai.NewChatModel(context.Background(), opts)
	if err != nil {
		return &Chat{configErr: errors.New(locales.ClientInitFailed())}
	}
	var summaryOptions []model.Option
	if strings.EqualFold(endpoint.Hostname(), "api.openai.com") {
		summaryOptions = append(summaryOptions, openai.WithMaxCompletionTokens(2048))
	} else {
		summaryOptions = append(summaryOptions, model.WithMaxTokens(2048))
	}
	if strings.EqualFold(endpoint.Hostname(), "api.deepseek.com") {
		summaryOptions = append(summaryOptions, openai.WithExtraFields(map[string]any{"thinking": map[string]string{"type": "disabled"}}))
	}
	buildExtendedAgent := func(ctx context.Context, name string, ext *agentExtensions) (adk.Agent, error) {
		return newConversationAgent(ctx, name, client, summaryOptions, config, network, ext)
	}
	buildAgent := func(ctx context.Context, name string) (adk.Agent, error) {
		return buildExtendedAgent(ctx, name, nil)
	}
	agent, err := buildAgent(context.Background(), conversationAgentName)
	if err != nil {
		return &Chat{configErr: errors.New(locales.AgentInitFailed())}
	}
	return &Chat{model: client, network: network, agent: agent, buildAgent: buildAgent, buildExtendedAgent: buildExtendedAgent, config: config}
}
