package harness

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Store interface {
	adk.SessionEventStore[*schema.AgenticMessage]
	adk.CheckPointStore
}

type Approval struct {
	CheckpointID string
	InterruptID  string
	Answer       string
}

// Session runs the Agent loop using the host's event log and checkpoint store.
type Session struct {
	runner *adk.TypedRunner[*schema.AgenticMessage]
	newID  func() string
}

type operationScopeKey struct{}

// OperationScope is stable when a turn resumes from its approval checkpoint.
func OperationScope(ctx context.Context) string {
	scope, _ := ctx.Value(operationScopeKey{}).(string)
	return scope
}

func NewSession(agent adk.TypedAgent[*schema.AgenticMessage], store Store, sessionID string, newID func() string) *Session {
	return &Session{
		newID: newID,
		runner: adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
			Agent: agent, EnableStreaming: true, SessionID: sessionID, SessionStore: store, CheckPointStore: store,
			SessionConfig: &adk.SessionConfig[*schema.AgenticMessage]{
				EventIDGenerator: func(context.Context, *adk.SessionEvent[*schema.AgenticMessage]) (string, error) { return newID(), nil },
			},
		}),
	}
}

func (s *Session) Run(ctx context.Context, input []*schema.AgenticMessage) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	id := s.newID()
	return s.runner.Run(context.WithValue(ctx, operationScopeKey{}, id), input, adk.WithCheckPointID(id))
}

func (s *Session) Resume(ctx context.Context, approval Approval) (*adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]], error) {
	ctx = context.WithValue(ctx, operationScopeKey{}, approval.CheckpointID)
	return s.runner.ResumeWithParams(ctx, approval.CheckpointID, &adk.ResumeParams{Targets: map[string]any{approval.InterruptID: approval.Answer}})
}
