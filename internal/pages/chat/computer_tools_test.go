package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/workspace"
)

type approvalComputer struct {
	workspace.ComputerDriver
	captures int
}

func (*approvalComputer) GetCapabilities(context.Context) (workspace.ComputerCapabilities, error) {
	return workspace.ComputerCapabilities{Screenshot: true}, nil
}

func (c *approvalComputer) Screenshot(context.Context) (workspace.Image, error) {
	c.captures++
	return workspace.Image{MIMEType: "image/png", Data: []byte("desktop image"), X: -100}, nil
}

func TestComputerScreenshotRequiresApproval(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Role string }
			Tools    []struct{ Function struct{ Name string } }
		}
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("read model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(raw, &request); err != nil || len(request.Messages) == 0 {
			t.Errorf("invalid model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(request.Messages) > 2 {
			if !strings.Contains(string(raw), "data:image/png;base64,ZGVza3RvcCBpbWFnZQ==") || !strings.Contains(string(raw), "(-100, 0)") {
				t.Errorf("model did not receive screenshot and origin: %.2000s", raw)
			}
			writeTestStream(w, "Finished")
			return
		}
		var hasCapture bool
		for _, item := range request.Tools {
			name := item.Function.Name
			if name == "computer_screenshot" {
				hasCapture = true
			} else if strings.HasPrefix(name, "computer_") {
				t.Errorf("unsupported computer tool offered: %s", name)
			}
		}
		if !hasCapture {
			t.Error("computer screenshot tool not offered")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"computer-tool\",\"type\":\"function\",\"function\":{\"name\":\"computer_screenshot\",\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n\n")
	}))
	defer model.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	driver := &approvalComputer{}
	agent := PersonalAgent{
		backend:  newChat(modelSettings{Protocol: openAIChat, BaseURL: model.URL, Model: "test"}),
		computer: computerBinding{driver: driver, identity: "desktop", label: "Local"},
	}
	ctx := localtools.WithAccess(t.Context(), localtools.NewAccess("", localtools.Ask))
	id := newID()
	emit := func(replyDelta) error { return nil }
	approval, err := agent.run(ctx, store, id, []*turn{userTurn("Capture the desktop")}, nil, "", emit)
	if err != nil || approval == nil || approval.Approval == nil || driver.captures != 0 {
		t.Fatalf("expected approval before capturing: %+v, %v, captures %d", approval, err, driver.captures)
	}
	approval, err = agent.run(ctx, store, id, nil, approval, localtools.AllowOnce, emit)
	if err != nil || approval != nil || driver.captures != 1 {
		t.Fatalf("expected one capture after approval: %+v, %v, captures %d", approval, err, driver.captures)
	}
}
