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
	const command = "echo ready > proof.txt"
	proof := filepath.Join(workspace, "proof.txt")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Role, Content string }
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
	backend := newChat(modelSettings{BaseURL: server.URL, Model: "test"})
	access := localtools.NewAccess(workspace, localtools.Ask)
	ctx := localtools.WithAccess(t.Context(), access)
	emit := func(replyDelta) error { return nil }

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
			access.SetMode(test.mode)
			id := newID()
			pending, err := backend.runSession(ctx, store, id, []*turn{userTurn("Run a command")}, nil, "", emit)
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
				pending, err = backend.runSession(ctx, store, id, nil, pending, test.answer, emit)
			}
			if err != nil || pending != nil {
				t.Fatalf("command run did not finish: %+v, %v", pending, err)
			}
			if test.wrote {
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
