package websearch

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino-ext/components/tool/searxng"
)

func newDuckDuckGo(ctx context.Context, client *http.Client, _ string, _ Options) (Provider, error) {
	engine, err := duckduckgo.NewSearch(ctx, &duckduckgo.Config{HTTPClient: client, MaxResults: ResultLimit})
	if err != nil {
		return nil, err
	}
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		response, err := engine.TextSearch(ctx, &duckduckgo.TextSearchRequest{Query: query})
		if err != nil {
			return nil, err
		}
		results := make([]Result, 0, len(response.Results))
		for _, hit := range response.Results {
			if hit != nil {
				results = append(results, Result{Title: hit.Title, URL: duckDuckGoURL(hit.URL), Snippet: hit.Summary})
			}
		}
		return results, nil
	}), nil
}

func duckDuckGoURL(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Hostname() == "duckduckgo.com" && strings.TrimSuffix(u.Path, "/") == "/l" {
		if url := u.Query().Get("uddg"); url != "" {
			return url
		}
	}
	return raw
}

func newSearXNG(_ context.Context, client *http.Client, endpoint string, _ Options) (Provider, error) {
	engine, err := searxng.NewClient(&searxng.ClientConfig{
		BaseUrl: endpoint, HttpClient: client, MaxRetries: 1,
	})
	if err != nil {
		return nil, err
	}
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		response, err := engine.Search(ctx, &searxng.SearchRequest{Query: query})
		if err != nil {
			// Eino reports an empty SearXNG result set as an error without an exported sentinel.
			if err.Error() == "no search results found" {
				return []Result{}, nil
			}
			return nil, err
		}
		results := make([]Result, 0, len(response.Results))
		for _, hit := range response.Results {
			if hit != nil {
				results = append(results, Result{Title: hit.Title, URL: hit.URL, Snippet: hit.Content})
			}
		}
		return results, nil
	}), nil
}
