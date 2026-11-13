package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestModelProtocolRequests(t *testing.T) {
	for _, tc := range []struct {
		name                                              string
		protocol                                          apiProtocol
		basePath, path, authHeader, authValue, tokenField string
	}{
		{"default Responses", "", "/v1", "/v1/responses", "Authorization", "Bearer test-key", "max_output_tokens"},
		{"Chat Completions", openAIChat, "/v1", "/v1/chat/completions", "Authorization", "Bearer test-key", "max_tokens"},
		{"Anthropic Messages", anthropicMessages, "", "/v1/messages", "X-Api-Key", "test-key", "max_tokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != tc.path || r.Header.Get(tc.authHeader) != tc.authValue {
					t.Errorf("unexpected request %s %s or authentication", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "test-model" || body["stream"] != true || body[tc.tokenField] != float64(256) {
					t.Errorf("unexpected request fields: %v", body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				switch tc.protocol {
				case openAIChat:
					writeTestStream(w, "OK")
				case anthropicMessages:
					fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"test-model\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"+
						"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"+
						"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"OK\"}}\n\n"+
						"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"+
						"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				default:
					if body["store"] != false || body["input"] == nil {
						t.Errorf("expected local history and stateless Responses request: %v", body)
					}
					fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"OK\"}\n\n")
				}
			}))
			defer server.Close()
			config := modelSettings{Protocol: tc.protocol, BaseURL: server.URL + tc.basePath, Model: "test-model", APIKey: "test-key", MaxTokens: new(256)}
			if err := checkConnection(t.Context(), config); err != nil {
				t.Fatal(err)
			}
			store, err := openLocalStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			backend := newChat(config)
			var reply string
			_, err = backend.runSession(t.Context(), store, newID(), []*turn{userTurn("Hello")}, nil, "", func(delta replyDelta) error {
				reply += delta.text
				return nil
			})
			if err != nil || reply != "OK" {
				t.Fatalf("reply %q, error %v", reply, err)
			}
		})
	}
}

func TestProviderProtocolSelectionPersists(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	app.loadSettings()
	app.openSettings()
	window := ui.NewTester(app.providerProtocolControl, 480, 240)
	click := func(label string) {
		t.Helper()
		if err := window.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if !window.HasText("Responses") {
		t.Fatal("Responses should be the default route")
	}
	click("Responses")
	click("Chat Completions")
	if app.form.protocol != openAIChat {
		t.Fatal("Chat Completions was not selected")
	}
	click("OpenAI")
	click("Anthropic")
	if app.form.protocol != anthropicMessages || !window.HasText("Messages") || window.HasText("Chat Completions") {
		t.Fatal("Anthropic should show its fixed Messages route")
	}
	app.form.baseURL = "https://api.anthropic.com"
	app.form.apiKey = "test-key"
	app.saveSettings()
	reopened := &chatApp{store: store}
	reopened.loadSettings()
	if reopened.settings.protocol() != anthropicMessages {
		t.Fatalf("reopened with protocol %q", reopened.settings.protocol())
	}
}
