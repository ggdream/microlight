package chat

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
)

func (app *chatApp) generate(chat *conversation, index int) {
	if app.active != nil || app.backend.configErr != nil {
		return
	}
	reply := &turn{message: message{Role: "assistant"}, State: "pending", StartedAt: time.Now()}
	if index < len(chat.Turns) {
		chat.sessionID = ""
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
	if answer != "" {
		question := reply.Clarifications[len(reply.Clarifications)-1]
		resume = &question
	} else if chat.sessionID == "" {
		chat.sessionID = newID()
		input = replyInput(chat.Turns[:index])
	} else {
		input = []*turn{chat.Turns[index-1]}
	}
	sessionID := chat.sessionID
	store := app.store
	chat.list.ScrollToEnd()
	ctx, cancel := context.WithCancel(context.Background())
	access := localtools.NewAccess(chat.workspace, chat.accessMode)
	ctx = localtools.WithAccess(ctx, access)
	app.active, app.running, app.cancel, app.runningAccess = chat, reply, cancel, access
	app.save(chat)

	app.providerError = ""
	temporaryBackend := backend != app.backend
	go func() {
		defer cancel()
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
				}
			})
			return nil
		}
		question, err := backend.runSession(ctx, store, sessionID, input, resume, answer, emit)
		finishedAt := time.Now()
		app.win.Update(func() {
			if app.running != reply {
				return
			}
			reply.FinishedAt = finishedAt
			reply.compressing = false
			reply.keepProcessPreview()
			if err == nil && resume != nil {
				answered := &reply.Clarifications[len(reply.Clarifications)-1]
				answered.CheckpointID, answered.InterruptID = "", ""
			}
			switch {
			case err != nil:
				chat.sessionID = ""
				log.Printf("对话执行失败（%T）", err)
				reply.State, reply.Error = "error", replyRequestFailed
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
			if strings.TrimSpace(reply.Content) != "" || len(reply.Clarifications) > 0 {
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
	if strings.TrimSpace(entry.Content) == "" && strings.TrimSpace(delta.text) != "" {
		entry.reasoningOpen = nil
	}
	entry.Content += delta.text
	entry.Reasoning += delta.reasoning
	entry.processPreview += delta.preview
	if delta.messageDone {
		entry.processPreview = ""
	}
	if delta.activity != nil {
		entry.Activity = append(entry.Activity, (*delta.activity)...)
	}
	if delta.toolFinished != "" {
		for i := len(entry.Activity) - 1; i >= 0; i-- {
			if entry.Activity[i].CallID == delta.toolFinished {
				entry.Activity[i].Pending = false
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
				break
			}
		}
	}
}

func (entry *turn) keepProcessPreview() {
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
