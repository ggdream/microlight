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

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/proxy"

	"github.com/cloudwego/eino-ext/components/model/agenticclaude"
	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/openai/openai-go/v3/responses"
)

const (
	defaultModel   = "gpt-5-mini"
	defaultBaseURL = "https://api.openai.com/v1"
)

type message struct {
	Role        string             `json:"role"`
	Content     string             `json:"content"`
	Attachments []*attachment.File `json:"-"`
}

type Chat struct {
	network            *proxy.Transport
	skillsHome         string
	model              model.AgenticModel
	configErr          error
	agent              adk.TypedAgent[*schema.AgenticMessage]
	buildExtendedAgent func(context.Context, *agentExtensions) (adk.TypedAgent[*schema.AgenticMessage], error)
	config             modelSettings
}

type replyDelta struct {
	interfaceUpdates []interfaceUpdate
	interfacePreview []interfaceUpdate
	text             string
	reasoning        string
	compression      compressionUpdate
	preview          string
	messageDone      bool
	activity         *[]timeline.Entry
	toolResult       *localtools.Result
	toolFinished     string
	toolOutput       *string
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

func modelMessages(messages []message) ([]*schema.AgenticMessage, error) {
	var history []*schema.AgenticMessage
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
			history = append(history, assistantMessage(m.Content))
		default:
			return nil, errors.New(locales.UnsupportedRole())
		}
	}
	return history, nil
}

func (chat *Chat) readReply(ctx context.Context, history []*schema.AgenticMessage, emit func(replyDelta) error) error {
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
		content, reasoning := generatedText(chunk)
		if content == "" && reasoning == "" {
			continue
		}
		hasText = hasText || strings.TrimSpace(content) != ""
		if err := emit(replyDelta{text: content, reasoning: reasoning}); err != nil {
			return streamError(ctx, err)
		}
	}
}

func (m message) userMessage() *schema.AgenticMessage {
	if len(m.Attachments) == 0 {
		return schema.UserAgenticMessage(m.Content)
	}
	text := m.Content
	var images []*schema.ContentBlock
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
		images = append(images, schema.NewContentBlock(&schema.UserInputImage{
			MIMEType: attachment.MIME, Base64Data: data,
		}))
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
	msg := schema.UserAgenticMessage(text)
	msg.ContentBlocks = append(msg.ContentBlocks, images...)
	return msg
}

func assistantMessage(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: text})}}
}

func generatedText(msg *schema.AgenticMessage) (string, string) {
	if msg == nil {
		return "", ""
	}
	var text, reasoning strings.Builder
	for _, block := range msg.ContentBlocks {
		if block.AssistantGenText != nil {
			text.WriteString(block.AssistantGenText.Text)
		}
		if block.Reasoning != nil {
			reasoning.WriteString(block.Reasoning.Text)
		}
	}
	return text.String(), reasoning.String()
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
	client, summaryOptions, err := newAgenticModel(context.Background(), config, endpoint.Hostname(),
		&http.Client{Transport: network, Timeout: 2 * time.Minute})
	if err != nil {
		return &Chat{configErr: errors.New(locales.ClientInitFailed())}
	}
	buildExtendedAgent := func(ctx context.Context, ext *agentExtensions) (adk.TypedAgent[*schema.AgenticMessage], error) {
		return newConversationAgent(ctx, conversationAgentName, client, summaryOptions, config, network, ext)
	}
	agent, err := buildExtendedAgent(context.Background(), nil)
	if err != nil {
		return &Chat{configErr: errors.New(locales.AgentInitFailed())}
	}
	return &Chat{model: client, network: network, agent: agent, buildExtendedAgent: buildExtendedAgent, config: config}
}

func newAgenticModel(ctx context.Context, config modelSettings, host string, httpClient *http.Client) (model.AgenticModel, []model.Option, error) {
	summary := []model.Option{model.WithMaxTokens(2048)}
	switch config.protocol() {
	case anthropicMessages:
		maxTokens := 4096
		if config.MaxTokens != nil {
			maxTokens = *config.MaxTokens
		}
		fields := make(map[string]any)
		if config.Temperature != nil {
			fields["temperature"] = *config.Temperature
		}
		if config.TopP != nil {
			fields["top_p"] = *config.TopP
		}
		client, err := agenticclaude.New(ctx, &agenticclaude.Config{
			APIKey: config.APIKey, BaseURL: config.BaseURL, Model: config.Model,
			MaxTokens: maxTokens, HTTPClient: httpClient, ExtraFields: fields,
		})
		return client, summary, err
	case openAIChat:
		fields := make(map[string]any)
		maxTokens := config.MaxTokens
		if !strings.EqualFold(host, "api.openai.com") && maxTokens != nil {
			fields["max_tokens"] = *maxTokens
			maxTokens = nil
		}
		if config.TopP != nil && *config.TopP == 0 {
			fields["top_p"] = 0
		}
		tokenField := "max_completion_tokens"
		if !strings.EqualFold(host, "api.openai.com") {
			tokenField = "max_tokens"
		}
		summaryFields := map[string]any{tokenField: 2048}
		if strings.EqualFold(host, "api.deepseek.com") {
			fields["thinking"] = map[string]string{"type": "enabled"}
			summaryFields["thinking"] = map[string]string{"type": "disabled"}
		}
		summary = []model.Option{agenticopenai.WithExtraFields(summaryFields)}
		client, err := agenticopenai.NewChatModel(ctx, &agenticopenai.ChatConfig{
			APIKey: config.APIKey, BaseURL: config.BaseURL, Model: config.Model,
			Temperature: config.Temperature, TopP: config.TopP, MaxCompletionTokens: maxTokens,
			HTTPClient: httpClient, ExtraFields: fields,
		})
		return client, summary, err
	default:
		client, err := agenticopenai.NewResponsesModel(ctx, &agenticopenai.ResponsesConfig{
			APIKey: config.APIKey, BaseURL: config.BaseURL, Model: config.Model,
			Temperature: config.Temperature, TopP: config.TopP, MaxTokens: config.MaxTokens,
			HTTPClient: httpClient, Store: new(false),
			Include: []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent},
		})
		return client, summary, err
	}
}
