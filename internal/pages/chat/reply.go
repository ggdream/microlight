package chat

import (
	"context"
	"errors"
	"fmt"
	"github.com/ggdream/microlight/internal/desktop"
	"log/slog"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/localtools"
)

func (app *chatApp) generate(chat *conversation, index int) {
	if chat.run != nil || app.backend.configErr != nil {
		return
	}
	reply := &turn{message: message{Role: "assistant"}, State: "pending", StartedAt: time.Now()}
	if index < len(chat.Turns) {
		for _, previous := range chat.Turns[index:] {
			previous.closeBrowser()
		}
		chat.sessionID = ""
		chat.projectedInterface = nil
	}
	chat.Turns = append(chat.Turns[:index], reply)
	app.runReply(chat, reply, index, "", app.backend, app.currentModelChoice())
}

func (chat *conversation) isRunning() bool {
	return chat != nil && chat.run != nil
}

func (chat *conversation) waitingReply() *turn {
	if chat != nil && len(chat.Turns) > 0 {
		last := chat.Turns[len(chat.Turns)-1]
		if last.State == "waiting" && last.canResume() {
			return last
		}
	}
	return nil
}

func (entry *turn) canResume() bool {
	return len(entry.Clarifications) > 0 && entry.Clarifications[len(entry.Clarifications)-1].CheckpointID != ""
}

func (app *chatApp) answerClarification(chat *conversation, reply *turn, answer string) {
	answer = strings.TrimSpace(answer)
	if answer == "" || chat.isRunning() || chat.waitingReply() != reply {
		return
	}
	question := &reply.Clarifications[len(reply.Clarifications)-1]
	if question.Approval != nil && answer != localtools.AllowOnce && answer != localtools.Deny {
		return
	}
	choice := question.Model
	backend, err := app.backendFor(choice)
	if err != nil {
		chat.runError = err.Error()
		return
	}
	if choice.ProviderID == "" {
		choice = app.currentModelChoice()
	}
	if chat.sessionModel != backend.sessionModelKey(choice) {
		chat.runError = locales.ChangedModelForResume()
		return
	}
	reply.excludeWaitingTime(time.Now())
	reply.FinishedAt = time.Time{}
	reply.State = "pending"
	reply.clarificationSurface = nil
	question.Answer = answer
	if question.ActivityOffset == nil {
		offset := len(reply.Activity)
		question.ActivityOffset = &offset
	}
	app.runReply(chat, reply, len(chat.Turns)-1, answer, backend, choice)
}

func (app *chatApp) runReply(chat *conversation, reply *turn, index int, answer string, backend *Chat, choice modelChoice) {
	release := mygo.Power.KeepRunning(locales.Generating())
	app.startReply(chat, reply, index, answer, backend, choice, app.win.Update, release)
}

func (app *chatApp) startReply(chat *conversation, reply *turn, index int, answer string, backend *Chat, choice modelChoice, update func(func()), release func()) {
	var input []*turn
	var resume *clarification
	sessionModel := backend.sessionModelKey(choice)
	calls := recoveryCalls(reply)
	reply.RecoveryModel = &choice
	if answer != "" {
		question := reply.Clarifications[len(reply.Clarifications)-1]
		resume = &question
	} else if chat.sessionID == "" || chat.sessionModel != sessionModel {
		// Provider reasoning and tool blocks may contain endpoint-specific data.
		chat.sessionID = newID()
		chat.sessionModel = sessionModel
		if calls != nil {
			input = recoveryInput(chat.Turns[:index], reply, calls)
		} else {
			input = replyInput(chat.Turns[:index])
		}
	} else {
		input = []*turn{chat.Turns[index-1]}
	}
	sessionID := chat.sessionID
	store := app.store
	chat.list.ScrollToEnd()
	parent := app.appContext()
	ctx, cancel := context.WithCancel(parent)
	if desktop.Desktop && backend.config.BrowserUse {
		if reply.browser == nil {
			reply.browser = localtools.NewBrowserSession(parent, backend.config.Proxy, backend.config.BrowserMode)
		}
		ctx = localtools.WithBrowserSession(ctx, reply.browser)
	} else {
		reply.closeBrowser()
	}
	browser := reply.browser
	ctx = context.WithValue(ctx, systemPromptKey{}, strings.TrimSpace(app.systemPrompt))
	access := localtools.NewAccess(chat.workspace, chat.accessMode)
	if chat.fileVersions == nil {
		chat.fileVersions = &localtools.FileVersions{}
	}
	access.TrackFiles(chat.fileVersions)
	ctx = localtools.WithBackground(ctx, app.tasks, chat.ID)
	if calls != nil {
		ctx = localtools.WithRecovery(ctx, calls)
		ctx = context.WithValue(ctx, recoveryPromptKey{}, chat.Turns[index-1].Content)
	}
	ctx = localtools.WithAccess(ctx, access)
	if backend.config.OCR.Enabled && index > 0 && chat.Turns[index-1].Role == "user" {
		ctx = localtools.WithOCRAttachments(ctx, chat.Turns[index-1].Attachments)
	}
	ctx = context.WithValue(ctx, interfaceContextKey{}, chat.interfaceRequest())
	run := &replyRun{reply: reply, cancel: cancel, access: access}
	chat.run = run
	chat.runError = ""
	app.save(chat)

	if app.selected() == chat {
		app.providerError = ""
	}
	temporaryBackend := backend != app.backend
	go func() {
		defer cancel()
		defer release()
		if temporaryBackend && backend.network != nil {
			defer backend.network.CloseIdleConnections()
		}
		emit := func(part replyDelta) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			update(func() {
				if parent.Err() == nil && chat.run == run {
					reply.appendDelta(part)
					if len(part.interfaceUpdates) != 0 {
						chat.commitInterface(reply, len(reply.Content)-len(part.text), part.interfaceUpdates)
					}
					if len(part.interfaceUpdates) != 0 || part.toolResult != nil || part.toolFinished != "" || part.activity != nil || part.metrics.Requests != 0 {
						app.save(chat)
					}
				}
			})
			return nil
		}
		question, err := backend.runSession(ctx, store, sessionID, input, resume, answer, emit)
		if browser != nil && (question == nil || err != nil) {
			browser.Close()
		}
		finishedAt := time.Now()
		update(func() {
			if parent.Err() != nil || chat.run != run {
				return
			}
			app.backgroundRefresh = time.Time{}
			if question == nil || err != nil {
				reply.browser = nil
			}
			reply.FinishedAt = finishedAt
			reply.activeTools = nil
			reply.compressing = false
			reply.keepProcessPreview()
			if err == nil && resume != nil {
				answered := &reply.Clarifications[len(reply.Clarifications)-1]
				answered.CheckpointID, answered.InterruptID = "", ""
			}
			switch {
			case err != nil:
				chat.sessionID = ""
				slog.Error("对话执行失败", "error_type", fmt.Sprintf("%T", err))
				reply.State, reply.Error = "error", replyRequestFailed
				var integration *extensionError
				if errors.Is(err, errMCPToolTimeout) {
					reply.Error = locales.McpToolTimedOut()
				} else if errors.As(err, &integration) {
					reply.Error = locales.SkillsReadFailed()
					if integration.server != "" {
						reply.Error = locales.McpConnectFailed() + " · " + integration.server
					}
				}
			case question != nil:
				offset := len(reply.Activity)
				question.ActivityOffset, question.Model = &offset, choice
				reply.Clarifications = append(reply.Clarifications, *question)
				reply.State = "waiting"
			default:
				reply.State = "done"
				reply.RecoveryOffset = nil
				reply.RecoveryModel = nil
			}
			chat.run = nil
			app.save(chat)
			app.notifyReply(chat.ID, reply)
			if reply.State == "done" {
				app.startNextQueued(chat)
			}
		})
	}()
}

func (chat *Chat) sessionModelKey(choice modelChoice) string {
	return strings.Join([]string{choice.ProviderID, string(chat.config.protocol()), chat.config.BaseURL, chat.config.Model}, "\n")
}

func replyInput(turns []*turn) []*turn {
	return append(historyTurns(turns[:len(turns)-1]), turns[len(turns)-1])
}

func historyTurns(turns []*turn) []*turn {
	input := make([]*turn, 0, len(turns))
	for i := 0; i+1 < len(turns); i += 2 {
		user, reply := turns[i], turns[i+1]
		switch reply.State {
		case "done":
			input = append(input, user, reply)
		case "steered":
			input = append(input, user)
			if strings.TrimSpace(reply.Content) != "" || len(reply.Interface) > 0 || len(reply.Clarifications) > 0 {
				input = append(input, reply)
			}
		}
	}
	return input
}

func (entry *turn) appendDelta(delta replyDelta) {
	entry.Metrics.add(delta.metrics)
	switch delta.compression {
	case compressionStarted:
		entry.compressing = true
	case compressionFinished:
		entry.compressing = false
	}
	if strings.TrimSpace(entry.Content) == "" && (strings.TrimSpace(delta.text) != "" || len(delta.interfaceUpdates) > 0) {
		entry.reasoningOpen = nil
	}
	entry.Content += delta.text
	entry.Reasoning += delta.reasoning
	entry.processPreview += delta.preview
	for _, update := range delta.interfacePreview {
		update.Offset += len(entry.Content)
		entry.interfacePreview = append(entry.interfacePreview, update)
	}
	if delta.messageDone {
		entry.processPreview = ""
		entry.interfacePreview = nil
	}
	if delta.activity != nil {
		entry.Activity = append(entry.Activity, (*delta.activity)...)
	}
	if delta.toolStarted != "" {
		if entry.activeTools == nil {
			entry.activeTools = make(map[string]bool)
		}
		entry.activeTools[delta.toolStarted] = true
	}
	if delta.toolFinished != "" {
		delete(entry.activeTools, delta.toolFinished)
		for i := len(entry.Activity) - 1; i >= 0; i-- {
			if entry.Activity[i].CallID == delta.toolFinished {
				activity := &entry.Activity[i]
				activity.Pending = false
				if localtools.RecordsMutation(activity.Tool) {
					activity.Receipt = delta.toolOutput
					if activity.Conflict || (activity.CommandResult != nil && !activity.CommandResult.Succeeded()) ||
						(activity.Receipt != nil && strings.HasPrefix(*activity.Receipt, "无法执行：")) {
						activity.Receipt = nil
					}
				}
				if activity.Output == nil && activity.Tool != timeline.AskUserTool {
					activity.Output = delta.toolOutput
				}
				break
			}
		}
	}
	if result := delta.toolResult; result != nil {
		for i := len(entry.Activity) - 1; i >= 0; i-- {
			if entry.Activity[i].CallID == result.CallID {
				entry.Activity[i].Conflict = result.Conflict
				entry.Activity[i].Diff = result.Diff
				entry.Activity[i].ResolvedTarget = result.Target
				entry.Activity[i].Model = result.Model
				entry.Activity[i].Output = &result.Text
				entry.Activity[i].CommandResult = result.Command
				break
			}
		}
	}
}

func (entry *turn) keepProcessPreview() {
	entry.interfacePreview = nil
	if text := strings.TrimSpace(entry.processPreview); text != "" {
		entry.Activity = append(entry.Activity, timeline.Entry{Text: text})
	}
	entry.processPreview = ""
}

func (entry *turn) excludeWaitingTime(now time.Time) {
	if !entry.StartedAt.IsZero() && !entry.FinishedAt.IsZero() {
		entry.StartedAt = entry.StartedAt.Add(max(0, now.Sub(entry.FinishedAt)))
	}
}
