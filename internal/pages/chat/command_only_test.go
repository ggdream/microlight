package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/workspace"
)

type commandOnlyRuntime struct {
	restrictedRuntime
	commands *commandOnlyShell
}

func (r commandOnlyRuntime) Commands() backgroundshell.RecoverableShell { return r.commands }

type commandOnlyShell struct {
	started string
}

func (s *commandOnlyShell) StartCommand(_ context.Context, request *backgroundshell.StartCommandRequest) (backgroundtool.Run, error) {
	if s.started != "" || request.Command != "printf selected-executor" {
		return nil, errors.New("unexpected command start")
	}
	s.started = request.TaskID
	return commandOnlyRun{}, nil
}

func (s *commandOnlyShell) RecoverCommand(_ context.Context, request *backgroundshell.RecoverCommandRequest) (backgroundtool.Run, error) {
	if request.TaskID != s.started || request.Command != "printf selected-executor" {
		return nil, errors.New("unexpected recover call")
	}
	return commandOnlyRun{}, nil
}

type commandOnlyRun struct{}

func (commandOnlyRun) Wait(context.Context) (*backgroundtool.Outcome, error) {
	return &backgroundtool.Outcome{Status: backgroundtask.StatusCompleted, Data: []byte(`{"Output":"selected-executor","ExitCode":0}`)}, nil
}

func (commandOnlyRun) Stop(context.Context) error { return nil }

func TestCommandOnlyExecutorRunsThroughApproval(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
			Tools []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Errorf("invalid model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		last := request.Messages[len(request.Messages)-1]
		if last.Role == "tool" {
			if !strings.Contains(string(last.Content), "selected-executor") {
				t.Errorf("unexpected tool output: %s", last.Content)
			}
			writeTestStream(w, "Finished")
			return
		}
		found := false
		for _, item := range request.Tools {
			switch item.Function.Name {
			case localtools.Execute:
				found = true
			case localtools.Read, localtools.Write, localtools.List:
				t.Errorf("file tool offered for a command-only executor: %s", item.Function.Name)
			}
		}
		if !found {
			t.Error("command tool missing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"command-only\",\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":%q}}]}}]}\n\ndata: [DONE]\n\n",
			localtools.Execute, `{"command":"printf selected-executor"}`)
	}))
	defer model.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	driver := &commandOnlyShell{}
	target := workspace.Target{Kind: "test-vm", Resource: "workspace-42"}
	connections := 0
	connect, err := workspace.NewConnector(map[string]workspace.ConnectFunc{
		target.Kind: func(_ context.Context, selected workspace.Target, secret string) (workspace.Runtime, error) {
			if selected != target || secret != "test-session-secret" {
				return nil, errors.New("wrong connector target or credential")
			}
			connections++
			return commandOnlyRuntime{commands: driver}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent := PersonalAgent{
		backend: newChat(modelSettings{Protocol: openAIChat, BaseURL: model.URL, Model: "test"}),
		target:  target,
		access:  localtools.NewAccess("", localtools.Ask),
		secret:  "test-session-secret",
		connect: connect,
	}
	sessionID := newID()
	emit := func(replyDelta) error { return nil }
	approval, err := agent.run(t.Context(), store, sessionID, []*turn{userTurn("Run the command")}, nil, "", emit)
	if err != nil || approval == nil || approval.Approval == nil || driver.started != "" {
		t.Fatalf("expected approval before launch: %+v, %v, task %q", approval, err, driver.started)
	}
	wantID := approval.CheckpointID + "/command-only"
	next, err := agent.run(t.Context(), store, sessionID, nil, approval, localtools.AllowOnce, emit)
	if err != nil || next != nil || driver.started != wantID {
		t.Fatalf("approval result: %+v, %v, task %q, want %q", next, err, driver.started, wantID)
	}
	if connections != 2 {
		t.Fatalf("connector opened %d times across approval, want 2", connections)
	}
	outcome, err := localtools.RecoverRemoteCommand(t.Context(), func(ctx context.Context) (workspace.Runtime, error) {
		return connect(ctx, target, agent.secret)
	}, localtools.RemoteCommand{ID: wantID, Command: "printf selected-executor"}, false)
	if err != nil || outcome.Status != backgroundtask.StatusCompleted || connections != 3 {
		t.Fatalf("reconnected command: %+v, %v, connections %d", outcome, err, connections)
	}
}
