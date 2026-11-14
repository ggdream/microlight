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

type approvalBrowser struct {
	workspace.BrowserDriver
	clicks int
}

func (b *approvalBrowser) Click(_ context.Context, tab, selector string) error {
	if tab != "tab1" || selector != "#run" {
		return fmt.Errorf("unexpected click %q %q", tab, selector)
	}
	b.clicks++
	return nil
}

func TestBrowserApprovalBindsDriver(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Role string }
			Tools    []struct {
				Function struct {
					Name       string
					Parameters struct{ Properties map[string]json.RawMessage }
				}
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Errorf("invalid model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Messages[len(request.Messages)-1].Role == "tool" {
			writeTestStream(w, "Finished")
			return
		}
		var hasClick bool
		for _, entry := range request.Tools {
			function := entry.Function
			if function.Name == "browser_click" {
				hasClick = function.Parameters.Properties["tab_id"] != nil && function.Parameters.Properties["selector"] != nil
			}
		}
		if !hasClick {
			t.Error("browser_click schema is missing tab_id or selector")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"browser-tool\",\"type\":\"function\",\"function\":{\"name\":\"browser_click\",\"arguments\":%q}}]}}]}\n\ndata: [DONE]\n\n",
			`{"tab_id":"tab1","selector":"#run"}`)
	}))
	defer model.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := &approvalBrowser{}
	second := &approvalBrowser{}
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: model.URL, Model: "test"})
	agent := PersonalAgent{
		backend: backend,
		browser: browserBinding{driver: first, identity: "first browser", label: "Local"},
	}
	ctx := localtools.WithAccess(t.Context(), localtools.NewAccess("", localtools.Ask))
	for _, outcome := range []struct {
		answer string
		swap   bool
		clicks int
	}{
		{answer: localtools.Deny},
		{answer: localtools.AllowOnce, swap: true},
		{answer: localtools.AllowOnce, clicks: 1},
	} {
		id := newID()
		emit := func(replyDelta) error { return nil }
		q, err := agent.run(ctx, store, id, []*turn{userTurn("Click the button")}, nil, "", emit)
		if err != nil || q == nil || q.Approval == nil || !strings.Contains(q.Approval.Preview, "#run") {
			t.Fatalf("expected browser approval, got %+v, %v", q, err)
		}
		if first.clicks != 0 {
			t.Fatal("browser was used before approval")
		}
		resumed := agent
		if outcome.swap {
			resumed.browser = browserBinding{driver: second, identity: "second browser", label: "Local"}
		}
		q, err = resumed.run(ctx, store, id, nil, q, outcome.answer, emit)
		if err != nil || q != nil || first.clicks != outcome.clicks || second.clicks != 0 {
			t.Fatalf("resume with %q, swap %v: question %+v, err %v, clicks %d and %d",
				outcome.answer, outcome.swap, q, err, first.clicks, second.clicks)
		}
	}
}

type restrictedRuntime struct {
	workspace.Runtime
}

func (restrictedRuntime) Environment() workspace.Environment {
	return workspace.Environment{OS: "linux"}
}
func (restrictedRuntime) Close() error { return nil }
func (restrictedRuntime) GetCapabilities(context.Context) (workspace.Capabilities, error) {
	return workspace.Capabilities{}, nil
}

func TestSelectedEnvironmentDeterminesOfferedTools(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("invalid model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		hasFetch := false
		for _, entry := range request.Tools {
			name := entry.Function.Name
			switch name {
			case localtools.Execute, localtools.Read, localtools.Write, localtools.List:
				t.Errorf("unsupported environment tool offered: %s", name)
			case localtools.Fetch:
				hasFetch = true
			}
			if strings.HasPrefix(name, "browser_") {
				t.Errorf("local browser leaked into selected environment: %s", name)
			}
		}
		if !hasFetch {
			t.Error("web fetch is missing")
		}
		writeTestStream(w, "Finished")
	}))
	defer model.Close()
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	agent := PersonalAgent{
		backend: newChat(modelSettings{Protocol: openAIChat, BaseURL: model.URL, Model: "test"}),
		target:  workspace.Target{Kind: "remote", Directory: "/workspace"},
		access:  localtools.NewAccess("", localtools.Ask),
		browser: browserBinding{driver: &approvalBrowser{}, identity: "local browser", label: "Local"},
		connect: func(context.Context, workspace.Target, string) (workspace.Runtime, error) {
			return restrictedRuntime{}, nil
		},
	}
	if q, err := agent.run(t.Context(), store, newID(), []*turn{userTurn("Show tools")}, nil, "", func(replyDelta) error { return nil }); err != nil || q != nil {
		t.Fatalf("session returned %+v, %v", q, err)
	}
}
