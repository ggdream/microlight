package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/websearch"
)

func TestWebSearchIsOptInAndRunsAfterApproval(t *testing.T) {
	const query = "Eino tool"
	var calls atomic.Int32
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != query || r.URL.Query().Get("format") != "json" {
			t.Errorf("invalid search request: %s", r.URL)
		}
		fmt.Fprint(w, `{"results":[{"title":"Eino","url":"https://example.test/eino","content":"Search fixture"}]}`)
	}))
	defer search.Close()

	var advertised atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []chatRequestMessage
			Tools    []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		hasSearch := slices.ContainsFunc(request.Tools, func(t struct{ Function struct{ Name string } }) bool {
			return t.Function.Name == localtools.WebSearch
		})
		advertised.Store(hasSearch)
		w.Header().Set("Content-Type", "text/event-stream")
		if !hasSearch {
			writeTestStream(w, "Disabled")
			return
		}
		for i := len(request.Messages) - 1; i >= 0; i-- {
			message := request.Messages[i]
			if message.Role == "tool" {
				if !strings.Contains(message.Content, "https://example.test/eino") {
					t.Errorf("model received incorrect search result: %s", message.Content)
				}
				writeTestStream(w, "Done")
				return
			}
			if message.Role == "user" {
				args := `{"query":"Eino tool"}`
				fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"search-1\",\"type\":\"function\",\"function\":{\"name\":\"web_search\",\"arguments\":%q}}]}}]}\n\ndata: [DONE]\n\n", args)
				return
			}
		}
		t.Error("missing user message")
	}))
	defer model.Close()

	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	skills := t.TempDir()
	config := modelSettings{Protocol: openAIChat, BaseURL: model.URL, Model: "test", AccessMode: localtools.Ask}
	config.WebSearch = localtools.SearchConfig{
		Provider: websearch.SearXNG,
		Options:  map[websearch.ID]websearch.Options{websearch.SearXNG: {BaseURL: search.URL}},
	}
	var body strings.Builder
	var started []string
	emit := func(delta replyDelta) error {
		body.WriteString(delta.text)
		if delta.toolStarted != "" {
			started = append(started, delta.toolStarted)
		}
		return nil
	}
	backend := newChat(config)
	backend.skillsHome = skills
	pending, err := backend.runSession(t.Context(), store, "disabled", []*turn{userTurn("Search")}, nil, "", emit)
	if err != nil || pending != nil || advertised.Load() || body.String() != "Disabled" {
		t.Fatalf("disabled search: %q, %+v, %v", body.String(), pending, err)
	}
	body.Reset()
	config.WebSearch.Enabled = true
	backend = newChat(config)
	backend.skillsHome = skills
	pending, err = backend.runSession(t.Context(), store, "enabled", []*turn{userTurn("Search")}, nil, "", emit)
	if err != nil || !advertised.Load() || pending == nil || pending.Approval == nil ||
		pending.Approval.Preview != query || !strings.Contains(pending.Approval.Target, search.URL) ||
		calls.Load() != 0 || len(started) != 0 {
		t.Fatalf("expected approval before search: %+v, %v, searches %d", pending, err, calls.Load())
	}
	pending, err = backend.runSession(t.Context(), store, "enabled", nil, pending, localtools.AllowOnce, emit)
	if err != nil || pending != nil || body.String() != "Done" || calls.Load() != 1 || !slices.Equal(started, []string{"search-1"}) {
		t.Fatalf("approved search: %q, %+v, %v, searches %d", body.String(), pending, err, calls.Load())
	}
}

func TestRetiredSearchProviderPreservesOtherSettings(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, test := range []struct {
		selected websearch.ID
		enabled  bool
		want     websearch.ID
	}{
		{selected: "bocha", enabled: false, want: websearch.DuckDuckGo},
		{selected: websearch.DeepSeek, enabled: true, want: websearch.DeepSeek},
	} {
		config := modelSettings{
			BaseURL: defaultBaseURL,
			Model:   defaultModel,
			APIKey:  "model-key",
			WebSearch: localtools.SearchConfig{
				Enabled:  true,
				Provider: test.selected,
				Options: map[websearch.ID]websearch.Options{
					"bocha":            {APIKey: "retired-key"},
					websearch.DeepSeek: {APIKey: "deepseek-key"},
				},
			},
		}
		if err := store.saveSettings(config); err != nil {
			t.Fatal(err)
		}
		saved, err := store.loadSettings()
		if err != nil || saved == nil {
			t.Fatalf("could not load settings with retired provider: %v", err)
		}
		if saved.APIKey != config.APIKey || saved.WebSearch.Enabled != test.enabled ||
			saved.WebSearch.Selected() != test.want || len(saved.WebSearch.Options) != 1 ||
			saved.WebSearch.Options[websearch.DeepSeek].APIKey != "deepseek-key" {
			t.Fatalf("migration incorrectly changed the settings for %q", test.selected)
		}
	}
}
