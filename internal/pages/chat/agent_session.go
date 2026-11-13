package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/session"
	skillset "github.com/ggdream/microlight/internal/skills"
	"github.com/ggdream/microlight/internal/storage"
	project "github.com/ggdream/microlight/internal/workspace"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/schema"
)

func (chat *Chat) runSession(ctx context.Context, store *localStore, sessionID string, input []*turn, resume *clarification, answer string, emit func(replyDelta) error) (*clarification, error) {
	if store == nil {
		return nil, errors.New(locales.DatabaseRequired())
	}
	agent := chat.agent
	// Earlier checkpoints reference the old root agent name in their run path.
	name := conversationAgentName
	if resume != nil && (resume.AgentName == "" || resume.AgentName == storage.LegacyAgentName) {
		name = storage.LegacyAgentName
	}
	workspace := localtools.Workspace(ctx, chat.config.Workspace)
	ctx = project.WithInstructions(ctx, workspace)
	var toolResults sync.Map
	ctx = localtools.WithResultReporter(ctx, func(result localtools.Result) {
		toolResults.Store(result.CallID, result)
	})
	skills := skillset.Discover(ctx, skillset.Roots(chat.skillsHome, workspace))
	if resume == nil && len(input) > 0 {
		ctx = withRequestedSkills(ctx, skills, input[len(input)-1].Content)
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
		agent, err = chat.buildExtendedAgent(ctx, name, &ext)
		if err != nil {
			return nil, err
		}
	} else if name == storage.LegacyAgentName {
		var err error
		agent, err = chat.buildAgent(ctx, name)
		if err != nil {
			return nil, err
		}
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: agent, EnableStreaming: true, SessionID: sessionID,
		SessionStore: store, CheckPointStore: store,
		SessionConfig: &adk.SessionConfig[*schema.Message]{
			EventIDGenerator: func(context.Context, *session.Event) (string, error) {
				return newID(), nil
			},
		},
	})
	var iter *adk.AsyncIterator[*adk.AgentEvent]
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
		iter = runner.Run(ctx, msgs, adk.WithCheckPointID(newID()))
	}
	question, err := readAgentReply(iter, &toolResults, skills, emit)
	if question != nil {
		question.AgentName = agent.Name(ctx)
	}
	return question, err
}

// A new session starts from the UI's original transcript. Subsequent calls
// pass just the new user turn and let Runner replay the event log.
func sessionInput(turns []*turn) ([]*schema.Message, error) {
	messages := make([]*schema.Message, 0, len(turns))
	for _, entry := range turns {
		switch entry.Role {
		case "user":
			if strings.TrimSpace(entry.Content) == "" && len(entry.Attachments) == 0 {
				return nil, errors.New(locales.EmptyMessage())
			}
			messages = append(messages, entry.message.userMessage())
		case "assistant":
			var content strings.Builder
			content.WriteString(entry.Content)
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
				msg := schema.AssistantMessage(content.String(), nil)
				msg.ReasoningContent = entry.Reasoning
				messages = append(messages, msg)
			}
		}
	}
	return messages, nil
}

func readAgentReply(iter *adk.AsyncIterator[*adk.AgentEvent], toolResults *sync.Map, skills *skillset.Catalog, emit func(replyDelta) error) (*clarification, error) {
	var question *clarification
	var failure error
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
			} else if action, ok := event.Action.CustomizedAction.(*summarization.CustomizedAction); ok && action != nil {
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
		var calls []*schema.Message
		var completedTool string
		collect := func(msg *schema.Message) {
			if msg == nil {
				return
			}
			if output.Role == schema.Tool && msg.ToolCallID != "" {
				completedTool = msg.ToolCallID
			}
			if output.Role != schema.Assistant || failure != nil {
				return
			}
			content.WriteString(msg.Content)
			reasoning.WriteString(msg.ReasoningContent)
			if len(msg.ToolCalls) != 0 {
				calls = append(calls, &schema.Message{ToolCalls: msg.ToolCalls})
			}
			if output.IsStreaming && msg.Content != "" {
				failure = emit(replyDelta{preview: msg.Content})
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
		if output.Role != schema.Assistant || failure != nil {
			if output.Role == schema.Tool {
				flushToolResults()
				if completedTool != "" && failure == nil {
					failure = emit(replyDelta{toolFinished: completedTool})
				}
			}
			continue
		}
		delta := replyDelta{messageDone: true, reasoning: reasoning.String()}
		if len(calls) == 0 {
			delta.text = content.String()
			hasText = hasText || strings.TrimSpace(delta.text) != ""
		} else {
			joined, err := schema.ConcatMessages(calls)
			if err != nil {
				failure = err
				continue
			}
			activity := make([]timeline.Entry, 0, len(joined.ToolCalls)+1)
			if text := strings.TrimSpace(content.String()); text != "" {
				activity = append(activity, timeline.Entry{Text: text})
			}
			for _, call := range joined.ToolCalls {
				item := timeline.DescribeToolCall(call)
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
	if failure == nil && !hasText && question == nil {
		failure = errors.New(locales.EmptyReply())
	}
	return question, failure
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
