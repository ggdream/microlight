package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/schema"
	"github.com/ggdream/microlight/pkg/a2ui"
)

// Compiler failures may not identify a surface. Keep them separate from strict
// renderer envelopes, which require a surface/function target. These messages
// contain diagnostics only; they do not alter the model's system instructions.
type interfaceFeedback struct {
	event     a2ui.RendererMessage
	err       error
	surfaceID string
}

func (feedback interfaceFeedback) modelInput() (*schema.AgenticMessage, error) {
	if feedback.err == nil {
		msg, err := interfaceEventInput(feedback.event)
		if err == nil {
			return msg, nil
		}
		feedback.err = fmt.Errorf("A2UI renderer event rejected: %w", err)
	}
	var diagnostic interface{ RendererMessage() a2ui.RendererMessage }
	if errors.As(feedback.err, &diagnostic) {
		if msg, err := interfaceEventInput(diagnostic.RendererMessage()); err == nil {
			return msg, nil
		}
	}
	raw, err := json.Marshal(struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		SurfaceID string `json:"surfaceId,omitempty"`
	}{Code: "A2UI_OUTPUT_ERROR", Message: feedback.err.Error(), SurfaceID: feedback.surfaceID})
	if err != nil {
		return nil, err
	}
	return schema.UserAgenticMessage(fmt.Sprintf("A2UI output error (JSON):\n%s", raw)), nil
}

// The UI can discover render, binding, media or action failures after the model
// stream ends. Retain them until the active follow-up or the next model request.
// Repainting the same surface must not repeatedly enqueue the same diagnostic.
type interfaceFeedbackQueue struct {
	mu       sync.Mutex
	pending  []interfaceFeedback
	reported map[string]bool
}

func (chat *conversation) interfaceFeedbackQueue() *interfaceFeedbackQueue {
	if chat.interfaceFeedback == nil {
		chat.interfaceFeedback = new(interfaceFeedbackQueue)
	}
	return chat.interfaceFeedback
}

func (q *interfaceFeedbackQueue) push(feedback interfaceFeedback) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(q.pending, feedback)
}

func (q *interfaceFeedbackQueue) report(scope string, feedback interfaceFeedback) {
	msg, err := feedback.modelInput()
	if err != nil {
		return
	}
	// Include structured targets/codes when deduplicating renderer callbacks.
	data, _ := json.Marshal(msg.ContentBlocks)
	key := scope + "/" + string(data)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.reported == nil {
		q.reported = make(map[string]bool)
	}
	if q.reported[key] {
		return
	}
	q.reported[key] = true
	q.pending = append(q.pending, feedback)
}

func (q *interfaceFeedbackQueue) take() []interfaceFeedback {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	pending := q.pending
	q.pending = nil
	return pending
}

func (q *interfaceFeedbackQueue) modelInputs() ([]*schema.AgenticMessage, error) {
	var messages []*schema.AgenticMessage
	for _, feedback := range q.take() {
		msg, err := feedback.modelInput()
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}
