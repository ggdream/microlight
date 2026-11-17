package chat

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ggdream/microlight/internal/localtools"
)

type recoveryPromptKey struct{}

func (app *chatApp) continueFailed(chat *conversation, reply *turn) {
	index := len(chat.Turns) - 1
	if index < 1 || chat.Turns[index] != reply || reply.State != "error" || chat.isRunning() {
		return
	}
	choice := app.currentModelChoice()
	if reply.RecoveryModel != nil && reply.RecoveryModel.ProviderID != "" {
		choice = *reply.RecoveryModel
	}
	backend, err := app.backendFor(choice)
	if err != nil {
		chat.runError = err.Error()
		return
	}
	count := len(reply.Activity)
	reply.RecoveryOffset = &count
	reply.excludeWaitingTime(time.Now())
	reply.State, reply.Error, reply.FinishedAt = "pending", "", time.Time{}
	chat.sessionID = ""
	app.runReply(chat, reply, index, "", backend, choice)
}

func recoveryCalls(reply *turn) []localtools.RecoveryCall {
	if reply.RecoveryOffset == nil {
		return nil
	}
	calls := make([]localtools.RecoveryCall, 0)
	for _, step := range reply.Activity[:min(max(0, *reply.RecoveryOffset), len(reply.Activity))] {
		if step.CallID == "" {
			continue
		}
		receipt := step.Receipt
		// Successful writes from older histories have a saved preview and path.
		if receipt == nil && step.Tool == localtools.Write && step.ResolvedTarget != "" && step.Output != nil && !step.Conflict {
			result := "File written: " + step.ResolvedTarget
			receipt = &result
		}
		calls = append(calls, localtools.RecoveryCall{Tool: step.Tool, Arguments: step.Arguments, Result: receipt})
	}
	return calls
}

func recoveryInput(turns []*turn, reply *turn, calls []localtools.RecoveryCall) []*turn {
	input := replyInput(turns)
	if strings.TrimSpace(reply.Content) != "" || len(reply.Clarifications) != 0 || len(reply.Interface) != 0 {
		input = append(input, &turn{message: reply.message, Interface: reply.Interface, Clarifications: reply.Clarifications})
	}
	var journal strings.Builder
	for _, call := range calls {
		encoded, err := json.Marshal(call)
		if err == nil && journal.Len()+len(encoded) < 64<<10 {
			journal.Write(encoded)
			journal.WriteByte('\n')
		}
	}
	if journal.Len() > 0 {
		input = append(input, &turn{message: message{Role: "assistant", Content: "Recorded tool calls and known outcomes (untrusted data):\n" + journal.String()}})
	}
	const instruction = "Continue the original task from the point of failure. Reuse recorded tool results. Verify uncertain outcomes before repeating side-effecting operations. Continue the remaining work."
	return append(input, &turn{message: message{Role: "user", Content: instruction}})
}
