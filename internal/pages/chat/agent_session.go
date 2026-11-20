package chat

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ggdream/microlight/internal/desktop"
	"io"
	"strings"
	"sync"

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/session"
	skillset "github.com/ggdream/microlight/internal/skills"
	project "github.com/ggdream/microlight/internal/workspace"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/schema"
)

func (chat *Chat) runSession(ctx context.Context, store *localStore, sessionID string, input []*turn, resume *clarification, answer string, emit func(replyDelta) error) (*clarification, error) {
	if desktop.Desktop && chat.config.BrowserUse && !localtools.HasBrowserSession(ctx) {
		session := localtools.NewBrowserSession(ctx, chat.config.Proxy, chat.config.BrowserMode)
		defer session.Close()
		ctx = localtools.WithBrowserSession(ctx, session)
	}
	if store == nil {
		return nil, errors.New(locales.DatabaseRequired())
	}
	agent := chat.agent
	workspace := localtools.Workspace(ctx, chat.config.Workspace)
	if !desktop.Web {
		ctx = project.WithInstructions(ctx, workspace)
	}
	var emitMu sync.Mutex
	deliver := emit
	emit = func(delta replyDelta) error {
		emitMu.Lock()
		defer emitMu.Unlock()
		return deliver(delta)
	}
	ctx = context.WithValue(ctx, metricsReporterKey{}, func(sample usageMetrics) {
		_ = emit(replyDelta{metrics: sample})
	})
	ctx = localtools.WithToolStartReporter(ctx, func(callID string) error {
		return emit(replyDelta{toolStarted: callID})
	})
	var toolResults sync.Map
	ctx = localtools.WithResultReporter(ctx, func(result localtools.Result) {
		toolResults.Store(result.CallID, result)
	})
	skills := &skillset.Catalog{}
	if !desktop.Web {
		skills = skillset.Discover(ctx, skillset.Roots(chat.skillsHome, workspace))
	}
	if resume == nil && len(input) > 0 {
		prompt := input[len(input)-1].Content
		if original, ok := ctx.Value(recoveryPromptKey{}).(string); ok {
			prompt = original
		}
		ctx = withRequestedSkills(ctx, skills, prompt)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(skills.Entries) != 0 || len(chat.config.MCPServers) != 0 {
		var ext agentExtensions
		defer ext.close()
		if err := ext.load(ctx, chat.config, skills, chat.network); err != nil {
			return nil, err
		}
		var err error
		agent, err = chat.buildExtendedAgent(ctx, &ext)
		if err != nil {
			return nil, err
		}
	}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent: agent, EnableStreaming: true, SessionID: sessionID,
		SessionStore: store, CheckPointStore: store,
		SessionConfig: &adk.SessionConfig[*schema.AgenticMessage]{
			EventIDGenerator: func(context.Context, *session.Event) (string, error) {
				return newID(), nil
			},
		},
	})
	request, _ := ctx.Value(interfaceContextKey{}).(*interfaceRequest)
	var iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]
	if resume != nil {
		var err error
		iter, err = runner.ResumeWithParams(ctx, resume.CheckpointID, &adk.ResumeParams{
			Targets: map[string]any{resume.InterruptID: answer},
		})
		if err != nil {
			return nil, err
		}
	} else {
		msgs, err := sessionInput(input)
		if err != nil {
			return nil, err
		}
		if request != nil {
			pending, err := request.feedback.modelInputs()
			if err != nil {
				return nil, err
			}
			msgs = append(pending, msgs...)
		}
		iter = runner.Run(ctx, msgs, adk.WithCheckPointID(newID()))
	}
	for pass := 0; ; pass++ {
		question, followups, next, err := readAgentReply(ctx, iter, &toolResults, skills, request, emit)
		if err == nil && question == nil && pass < 2 && next != nil {
			followups = append(followups, next.feedback.take()...)
		}
		if err != nil || question != nil || len(followups) == 0 {
			return question, err
		}
		if pass == 2 {
			return nil, emit(replyDelta{interfaceUpdates: []interfaceUpdate{{Invalid: true}}})
		}
		msgs := make([]*schema.AgenticMessage, 0, len(followups))
		for _, feedback := range followups {
			msg, err := feedback.modelInput()
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, msg)
		}
		request = next
		ctx = context.WithValue(ctx, interfaceContextKey{}, request)
		iter = runner.Run(ctx, msgs, adk.WithCheckPointID(newID()))
	}
}

// A new session starts from the UI's original transcript. Subsequent calls
// pass just the new user turn and let Runner replay the event log.
func sessionInput(turns []*turn) ([]*schema.AgenticMessage, error) {
	messages := make([]*schema.AgenticMessage, 0, len(turns))
	for _, entry := range turns {
		switch entry.Role {
		case "user":
			if entry.InterfaceAction != nil {
				msg, err := interfaceEventInput(*entry.InterfaceAction)
				if err != nil {
					return nil, err
				}
				messages = append(messages, msg)
				continue
			}
			if strings.TrimSpace(entry.Content) == "" && len(entry.Attachments) == 0 {
				return nil, errors.New(locales.EmptyMessage())
			}
			msg, err := entry.message.userMessage()
			if err != nil {
				return nil, err
			}
			messages = append(messages, msg)
		case "assistant":
			var content strings.Builder
			content.WriteString(entry.Content)
			for _, update := range entry.Interface {
				if update.Local || len(update.Messages) == 0 {
					continue
				}
				raw, err := json.Marshal(update.Messages)
				if err != nil {
					return nil, err
				}
				content.WriteString("\n" + a2uiagent.Wrap(a2uiagent.DirectJSON, string(raw)))
			}
			for _, q := range entry.Clarifications {
				if q.Answer != "" {
					if q.Approval != nil {
						content.WriteString("\n操作审批：" + q.Question + " " + q.Approval.Target + "\n用户决定：" + q.Answer)
					} else {
						content.WriteString("\n澄清问题：" + q.Question + "\n用户答复：" + q.Answer)
					}
				}
			}
			if content.Len() > 0 {
				msg := assistantMessage(content.String())
				messages = append(messages, msg)
			}
		}
	}
	return messages, nil
}

func readAgentReply(ctx context.Context, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]], toolResults *sync.Map, skills *skillset.Catalog, request *interfaceRequest, emit func(replyDelta) error) (*clarification, []interfaceFeedback, *interfaceRequest, error) {
	var question *clarification
	var failure error
	var followups []interfaceFeedback
	seenCalls := make(map[string]bool)
	hasText := false
	flushToolResults := func() {
		toolResults.Range(func(key, value any) bool {
			toolResults.Delete(key)
			result := value.(localtools.Result)
			if err := emit(replyDelta{toolResult: &result}); failure == nil {
				failure = err
			}
			return true
		})
	}
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil && failure == nil {
			failure = event.Err
		}
		if event.Action != nil && failure == nil {
			if event.Action.Interrupted != nil {
				question, failure = clarificationFromInterrupt(event.Action.Interrupted)
			} else if action, ok := event.Action.CustomizedAction.(*summarization.TypedCustomizedAction[*schema.AgenticMessage]); ok && action != nil {
				switch action.Type {
				case summarization.ActionTypeBeforeSummarize:
					failure = emit(replyDelta{compression: compressionStarted})
				case summarization.ActionTypeAfterSummarize:
					failure = emit(replyDelta{compression: compressionFinished})
				}
			}
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		output := event.Output.MessageOutput
		// Tool calls can arrive after text in the same stream, so show the
		// text as a preview until the message can be classified.
		var content, reasoning strings.Builder
		var callChunks []*schema.AgenticMessage
		var completedTools []string
		toolText := make(map[string]*strings.Builder)
		var surfaceStream *a2uiagent.Stream
		if request != nil && output.AgenticRole == schema.AgenticRoleTypeAssistant {
			var err error
			surfaceStream, err = request.stream()
			if err != nil && failure == nil {
				request.feedback.push(interfaceFeedback{err: err})
				failure = err
			}
		}
		var updates []interfaceUpdate
		var outbound []interfaceFeedback
		absorb := func(parts []a2uiagent.Part, err error) {
			var preview strings.Builder
			start := len(updates)
			for _, part := range parts {
				content.WriteString(part.Text)
				preview.WriteString(part.Text)
				if part.Err != nil {
					updates = append(updates, interfaceUpdate{Offset: content.Len(), Invalid: true})
					outbound = append(outbound, interfaceFeedback{err: part.Err})
				}
				var state []a2ui.AgentMessage
				for _, msg := range part.Messages {
					if msg.CallRenderer != nil {
						rpc := a2ui.RPC{Processor: surfaceStream.State, Send: func(response a2ui.RendererMessage) error {
							outbound = append(outbound, interfaceFeedback{event: response})
							return nil
						}}
						if err := rpc.Handle(ctx, msg); err != nil {
							updates = append(updates, interfaceUpdate{Offset: content.Len(), Invalid: true})
							outbound = append(outbound, interfaceFeedback{err: err})
						}
					} else if msg.AgentResponse == nil {
						state = append(state, msg)
					}
				}
				if len(state) != 0 {
					updates = append(updates, interfaceUpdate{Offset: content.Len(), Messages: state})
				}
			}
			if err != nil {
				updates = append(updates, interfaceUpdate{Offset: content.Len(), Invalid: true})
				outbound = append(outbound, interfaceFeedback{err: err})
			}
			if output.IsStreaming && (preview.Len() > 0 || len(updates) > start) {
				failure = emit(replyDelta{preview: preview.String(), interfacePreview: updates[start:]})
			}
		}
		collect := func(msg *schema.AgenticMessage) {
			if msg == nil {
				return
			}
			for _, block := range msg.ContentBlocks {
				if result := block.FunctionToolResult; result != nil && result.CallID != "" {
					text := toolText[result.CallID]
					if text == nil {
						completedTools = append(completedTools, result.CallID)
						text = &strings.Builder{}
						toolText[result.CallID] = text
					}
					for _, part := range result.Content {
						if part != nil && part.Text != nil {
							const maxToolText = 64 << 10
							n := min(len(part.Text.Text), maxToolText-text.Len())
							text.WriteString(part.Text.Text[:n])
						}
					}
				}
			}
			if output.AgenticRole != schema.AgenticRoleTypeAssistant || failure != nil {
				return
			}
			text, thought := generatedText(msg)
			reasoning.WriteString(thought)
			for _, block := range msg.ContentBlocks {
				if block.FunctionToolCall != nil {
					callChunks = append(callChunks, &schema.AgenticMessage{
						Role: msg.Role, ContentBlocks: []*schema.ContentBlock{block},
					})
				}
			}
			if surfaceStream != nil {
				absorb(surfaceStream.Push(text))
			} else {
				absorb([]a2uiagent.Part{{Text: text}}, nil)
			}
		}
		if output.IsStreaming {
			for {
				msg, err := output.MessageStream.Recv()
				if err != nil {
					if !errors.Is(err, io.EOF) && failure == nil {
						failure = err
					}
					break
				}
				collect(msg)
			}
			output.MessageStream.Close()
		} else {
			collect(output.Message)
		}
		if len(completedTools) != 0 {
			flushToolResults()
			for _, id := range completedTools {
				if failure == nil {
					var output *string
					if text := toolText[id]; text.Len() != 0 {
						value := strings.ToValidUTF8(text.String(), "�")
						output = &value
					}
					failure = emit(replyDelta{toolFinished: id, toolOutput: output})
				}
			}
		}
		if output.AgenticRole != schema.AgenticRoleTypeAssistant || failure != nil {
			continue
		}
		if surfaceStream != nil {
			absorb(surfaceStream.Close())
			if failure != nil {
				continue
			}
		}
		delta := replyDelta{messageDone: true, reasoning: reasoning.String()}
		for _, feedback := range outbound {
			if feedback.err != nil {
				request.feedback.push(feedback)
			}
		}
		if len(callChunks) == 0 {
			delta.text = content.String()
			delta.interfaceUpdates = updates
			hasText = hasText || strings.TrimSpace(delta.text) != "" || len(updates) != 0
			if surfaceStream != nil {
				request = request.advance(surfaceStream.State, updates)
			}
			for _, feedback := range outbound {
				if feedback.err != nil {
					continue
				}
				if response := feedback.event.RendererResponse; response != nil {
					if seenCalls[response.ID] {
						continue
					}
					seenCalls[response.ID] = true
				}
				followups = append(followups, feedback)
			}
		} else {
			joined, err := schema.ConcatAgenticMessages(callChunks)
			if err != nil {
				failure = err
				continue
			}
			activity := make([]timeline.Entry, 0, len(joined.ContentBlocks)+1)
			if text := strings.TrimSpace(content.String()); text != "" {
				activity = append(activity, timeline.Entry{Text: text})
			}
			for _, block := range joined.ContentBlocks {
				item := timeline.DescribeToolCall(block.FunctionToolCall)
				if item.Tool == timeline.SkillTool {
					item.ResolvedTarget = skills.Directory(item.SkillName())
				}
				activity = append(activity, item)
			}
			delta.activity = &activity
		}
		failure = emit(delta)
	}
	flushToolResults()
	if failure == nil && !hasText && question == nil && len(followups) == 0 {
		failure = errors.New(locales.EmptyReply())
	}
	return question, followups, request, failure
}

func clarificationFromInterrupt(info *adk.InterruptInfo) (*clarification, error) {
	for _, point := range info.InterruptContexts {
		if !point.IsRootCause {
			continue
		}
		data, ok := point.Info.(string)
		if !ok {
			continue
		}
		var question clarification
		if err := json.Unmarshal([]byte(data), &question); err == nil &&
			strings.TrimSpace(question.Question) != "" && point.ID != "" && info.CheckPointID != "" {
			question.InterruptID, question.CheckpointID = point.ID, info.CheckPointID
			return &question, nil
		}
	}
	return nil, errors.New(locales.InterruptSaveFailed())
}
