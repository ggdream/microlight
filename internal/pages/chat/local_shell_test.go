package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ggdream/microlight/internal/localtools"
)

func TestDeepExecuteRespectsConversationApproval(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const command = "echo ready > proof.txt; echo shell-output"
	proof := filepath.Join(workspace, "proof.txt")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []chatRequestMessage
			Tools    []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Errorf("invalid request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Messages[len(request.Messages)-1].Role == "tool" {
			writeTestStream(w, "Finished")
			return
		}
		found := false
		for _, tool := range request.Tools {
			found = found || tool.Function.Name == "execute"
		}
		if !found {
			t.Error("DeepAgent did not register execute")
		}
		args, _ := json.Marshal(map[string]string{"command": command})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"command-1\",\"type\":\"function\",\"function\":{\"name\":\"execute\",\"arguments\":%q}}]}}]}\n\ndata: [DONE]\n\n", args)
	}))
	defer server.Close()

	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test"})
	access := localtools.NewAccess(workspace, localtools.Ask)
	ctx := localtools.WithAccess(t.Context(), access)
	agent := PersonalAgent{backend: backend, access: access}

	for _, test := range []struct {
		mode   localtools.Mode
		answer string
		wrote  bool
	}{
		{localtools.Ask, localtools.Deny, false},
		{localtools.Smart, localtools.AllowOnce, true},
		{localtools.Full, "", true},
	} {
		t.Run(string(test.mode), func(t *testing.T) {
			var reply turn
			emit := func(delta replyDelta) error { reply.appendDelta(delta); return nil }
			access.SetMode(test.mode)
			id := newID()
			pending, err := agent.run(ctx, store, id, []*turn{userTurn("Run a command")}, nil, "", emit)
			if err != nil {
				t.Fatal(err)
			}
			if test.answer != "" {
				if pending == nil || pending.Approval == nil || pending.Approval.Preview != command || pending.Approval.Target != workspace {
					t.Fatalf("expected approval with exact command and directory: %+v", pending)
				}
				if _, err := os.Stat(proof); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("command ran before approval: %v", err)
				}
				pending, err = agent.run(ctx, store, id, nil, pending, test.answer, emit)
			}
			if err != nil || pending != nil {
				t.Fatalf("command run did not finish: %+v, %v", pending, err)
			}
			if test.wrote {
				if len(reply.Activity) == 0 {
					t.Fatal("missing command activity")
				}
				activity := reply.Activity[0]
				if activity.Output == nil || strings.TrimSpace(*activity.Output) != "shell-output" || activity.Pending {
					t.Fatalf("unexpected command activity: %+v", activity)
				}
				if result := activity.CommandResult; result == nil || result.ExitCode == nil || *result.ExitCode != 0 {
					t.Fatalf("expected the shell exit code, got %+v", result)
				}
				data, err := os.ReadFile(proof)
				if err != nil || !strings.Contains(string(data), "ready") {
					// Windows PowerShell writes UTF-16 when redirecting text.
					if err != nil || !strings.Contains(strings.ReplaceAll(string(data), "\x00", ""), "ready") {
						t.Fatalf("command did not write in the workspace: %q, %v", data, err)
					}
				}
				if err := os.Remove(proof); err != nil {
					t.Fatal(err)
				}
			} else if _, err := os.Stat(proof); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("denied command ran: %v", err)
			}
		})
	}
}
