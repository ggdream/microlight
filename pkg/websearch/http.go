package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxSearchResponse = 2 << 20

type searchTransport struct {
	http.RoundTripper
	id         ID
	credential string
}

// Buffer and close the upstream body even when an SDK leaves closing it to the caller.
func (transport searchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := transport.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSearchResponse+1))
	if err != nil {
		return nil, err
	}
	httpFailed := resp.StatusCode != http.StatusOK
	if len(body) > maxSearchResponse {
		if httpFailed {
			return nil, responseFailure(transport.id, resp.StatusCode, upstreamError{}, transport.credential)
		}
		return nil, &Failure{Kind: "invalid_response", Message: fmt.Sprintf("The search response exceeds %d bytes.", maxSearchResponse)}
	}
	if upstream, failed := parseSearchError(body, httpFailed, transport.id); failed {
		return nil, responseFailure(transport.id, resp.StatusCode, upstream, transport.credential)
	}
	buffered := *resp
	buffered.Body = io.NopCloser(bytes.NewReader(body))
	return &buffered, nil
}

func postJSON(ctx context.Context, client *http.Client, endpoint, key string, input, output any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, output)
}

func newDeepSeek(_ context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		input := struct {
			Query string `json:"query"`
			Count int    `json:"count"`
		}{Query: query, Count: ResultLimit}
		var output struct {
			Results *[]struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Summary string `json:"summary"`
			} `json:"results"`
		}
		if err := postJSON(ctx, client, endpoint, options.APIKey, input, &output); err != nil {
			return nil, err
		}
		if output.Results == nil {
			return nil, errors.New("missing results from DeepSeek")
		}
		results := make([]Result, 0, len(*output.Results))
		for _, hit := range *output.Results {
			results = append(results, Result{Title: hit.Title, URL: hit.URL, Snippet: hit.Summary})
		}
		return results, nil
	}), nil
}

func newBaidu(_ context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type resource struct {
		Type string `json:"type"`
		TopK int    `json:"top_k"`
	}
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		input := struct {
			Messages []message  `json:"messages"`
			Source   string     `json:"search_source"`
			Edition  string     `json:"edition"`
			Filter   []resource `json:"resource_type_filter"`
		}{
			Messages: []message{{Role: "user", Content: query}},
			Source:   "baidu_search_v2", Edition: "standard", Filter: []resource{{Type: "web", TopK: ResultLimit}},
		}
		var output struct {
			References *[]Result `json:"references"`
		}
		if err := postJSON(ctx, client, endpoint, options.APIKey, input, &output); err != nil {
			return nil, err
		}
		if output.References == nil {
			return nil, errors.New("missing references from Baidu")
		}
		return *output.References, nil
	}), nil
}
