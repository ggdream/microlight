package localtools

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	search "github.com/ggdream/microlight/pkg/websearch"
)

func TestSearchToolDistinguishesProviderErrorFromEmptyResults(t *testing.T) {
	const query = `{"query":"微光"}`
	for _, tc := range []struct {
		name string
		body string
		code string
	}{
		{"provider error", `{"code":"InternalError","message":"Search index failed"}`, "InternalError"},
		{"empty results", `{"references":[]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, _, err := search.NewProvider(t.Context(), search.Baidu, search.Options{APIKey: "fake-key"}, &http.Client{
				Transport: searchTestTransport(func(req *http.Request) (*http.Response, error) {
					req.Body.Close()
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (searchTool{provider: provider, name: "Baidu"}).InvokableRun(t.Context(), query)
			if err != nil {
				t.Fatal(err)
			}
			if tc.code == "" {
				if want := `{"provider":"Baidu","results":[]}`; result != want {
					t.Fatalf("empty results = %s, want %s", result, want)
				}
				return
			}
			var decoded struct {
				Status, Provider string
				Error            search.Failure
			}
			if err := json.Unmarshal([]byte(result), &decoded); err != nil || decoded.Status != "error" ||
				decoded.Provider != "Baidu" || decoded.Error.Code != tc.code ||
				decoded.Error.Detail != "Search index failed" || !decoded.Error.Retryable {
				t.Fatalf("invalid model-facing failure: %s (%v)", result, err)
			}
		})
	}
}

type searchTestTransport func(*http.Request) (*http.Response, error)

func (transport searchTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}
