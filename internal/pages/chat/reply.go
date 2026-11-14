package chat

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/harness"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/workspace"
)

func (app *chatApp) generate(chat *conversation, index int) {
	if app.active != nil || app.backend.configErr != nil {
		return
	}
	reply := &turn{message: message{Role: "assistant"}, State: "pending", StartedAt: time.Now()}
	if index < len(chat.Turns) {
		chat.sessionID = ""
		chat.projectedInterface = nil
	}
	chat.Turns = append(chat.Turns[:index], reply)
	app.runReply(chat, reply, index, "", app.backend, app.currentModelChoice())
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

func (app *chatApp) resumeRemoteReply(chat *conversation, reply *turn) {
	if app.active != nil || reply.recoveringRemote || reply.resumeCommand == nil || !reply.canResume() ||
		len(chat.Turns) == 0 || chat.Turns[len(chat.Turns)-1] != reply {
		return
	}
	question := reply.Clarifications[len(reply.Clarifications)-1]
	choice := question.Model
	backend, err := app.backendFor(choice)
	if err != nil {
		reply.Error = err.Error()
		return
	}
	if choice.ProviderID == "" {
		choice = app.currentModelChoice()
	}
	if chat.sessionModel != backend.sessionModelKey(choice) {
		reply.Error = locales.ChangedModelForResume()
		return
	}
	reply.Error, reply.State = "", "pending"
	app.runReply(chat, reply, len(chat.Turns)-1, question.Answer, backend, choice)
}

func (app *chatApp) answerClarification(chat *conversation, reply *turn, answer string) {
	answer = strings.TrimSpace(answer)
	if answer == "" || app.active != nil || chat.waitingReply() != reply {
		return
	}
	question := &reply.Clarifications[len(reply.Clarifications)-1]
	if question.Approval != nil && answer != localtools.AllowOnce && answer != localtools.Deny {
		return
	}
	choice := question.Model
	backend, err := app.backendFor(choice)
	if err != nil {
		app.providerError = err.Error()
		return
	}
	if choice.ProviderID == "" {
		choice = app.currentModelChoice()
	}
	if chat.sessionModel != backend.sessionModelKey(choice) {
		app.providerError = locales.ChangedModelForResume()
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
	var input []*turn
	var resume *clarification
	sessionModel := backend.sessionModelKey(choice)
	if answer != "" {
		question := reply.Clarifications[len(reply.Clarifications)-1]
		resume = &question
	} else if chat.sessionID == "" || chat.sessionModel != sessionModel {
		// Provider reasoning and tool blocks may contain endpoint-specific data.
		chat.sessionID = newID()
		chat.sessionModel = sessionModel
		input = replyInput(chat.Turns[:index])
	} else {
		input = []*turn{chat.Turns[index-1]}
	}
	sessionID, chatID, branch := chat.sessionID, chat.ID, chat.branchID
	store := app.store
	chat.list.ScrollToEnd()
	ctx, cancel := context.WithCancelCause(context.Background())
	access := localtools.NewAccess(chat.workspace, chat.accessMode)
	target := chat.target
	var browser browserBinding
	var computer computerBinding
	if target.Kind == "" {
		computer = computerBinding{driver: workspace.NewLocalComputer(), identity: app.desktopID, label: target.Label()}
		if chat.browser == nil {
			chat.browser = workspace.NewLocalBrowser()
			chat.browserID = newID()
		}
		if chat.browser != nil {
			browser = browserBinding{driver: chat.browser, identity: chat.browserID, label: target.Label()}
		}
	}
	secret := app.workspaceSecrets[target.Credential]
	connect := app.workspaceConnector()
	ctx = localtools.WithAccess(ctx, access)
	ctx = context.WithValue(ctx, interfaceContextKey{}, chat.interfaceRequest())
	if record := reply.resumeCommand; record != nil {
		if record.beforeJournal {
			ctx = localtools.WithCommandCheckpoint(ctx, record.checkpoint)
		} else {
			ctx = localtools.WithCommandReattachment(ctx, record.RemoteCommand, record.outcome)
		}
	}
	if store != nil && target.Kind != "" {
		ctx = localtools.WithCommandJournal(ctx, store.commandJournal(chatID, branch, target.Identity(), index))
	}
	recoveringReply := reply.resumeCommand != nil
	app.active, app.running, app.cancel, app.runningAccess = chat, reply, func() { cancel(localtools.ErrStopReply) }, access
	app.save(chat)

	app.providerError = ""
	temporaryBackend := backend != app.backend
	go func() {
		defer cancel(nil)
		if temporaryBackend && backend.network != nil {
			defer backend.network.CloseIdleConnections()
		}
		emit := func(part replyDelta) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			app.win.Update(func() {
				if app.running == reply {
					reply.appendDelta(part)
					if len(part.interfaceUpdates) != 0 {
						chat.commitInterface(reply, len(reply.Content)-len(part.text), part.interfaceUpdates)
						app.save(chat)
					}
				}
			})
			return nil
		}
		agent := PersonalAgent{backend: backend, target: target, access: access, secret: secret, browser: browser, computer: computer, connect: connect}
		var sessionStore harness.Store
		if store != nil {
			sessionStore = &store.Store
		}
		question, err := agent.run(ctx, sessionStore, sessionID, input, resume, answer, emit)
		var saved []savedRemoteCommand
		if store != nil && target.Kind != "" {
			saved, _ = store.loadRemoteCommands(chatID, branch, index)
		}
		finishedAt := time.Now()
		app.win.Update(func() {
			current := chat.branchID == branch && index < len(chat.Turns) && chat.Turns[index] == reply
			if current && len(saved) > 0 {
				reply.applyRemoteCommands(saved)
			}
			if app.running != reply {
				if current && len(saved) > 0 {
					app.save(chat)
				}
				if !current && len(saved) > 0 {
					app.refreshDetachedCommands(chat)
				}
				return
			}
			reply.FinishedAt = finishedAt
			reply.compressing = false
			reply.keepProcessPreview()
			if err == nil || !recoveringReply {
				reply.resumeCommand = nil
			}
			if err == nil && resume != nil {
				answered := &reply.Clarifications[len(reply.Clarifications)-1]
				answered.CheckpointID, answered.InterruptID = "", ""
			}
			switch {
			case err != nil:
				reply.resumeCommand = store.recoverableReplyCommand(chat, reply)
				if reply.resumeCommand == nil {
					chat.sessionID = ""
				}
				log.Printf("对话执行失败（%T）", err)
				reply.State, reply.Error = "error", replyRequestFailed
				var workspaceErr *workspaceConnectionError
				if errors.As(err, &workspaceErr) {
					reply.Error = workspaceErr.Error()
				}
				var integration *extensionError
				if errors.As(err, &integration) {
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
			}
			app.active, app.running, app.cancel, app.runningAccess = nil, nil, nil, nil
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
	if delta.toolFinished != "" {
		for i := len(entry.Activity) - 1; i >= 0; i-- {
			if entry.Activity[i].CallID == delta.toolFinished {
				activity := &entry.Activity[i]
				activity.Pending = false
				if activity.Output == nil && activity.RunsCommand() {
					activity.Output = delta.toolOutput
				}
				break
			}
		}
	}
	if result := delta.toolResult; result != nil {
		for i := len(entry.Activity) - 1; i >= 0; i-- {
			if entry.Activity[i].CallID == result.CallID {
				entry.Activity[i].Diff = result.Diff
				entry.Activity[i].ResolvedTarget = result.Target
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
