package websearch

import (
	"context"
	"errors"
	"net/http"
)

func newTavily(_ context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		input := struct {
			Query string `json:"query"`
			Depth string `json:"search_depth"`
			Limit int    `json:"max_results"`
		}{Query: query, Depth: "basic", Limit: ResultLimit}
		var output struct {
			Results *[]struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
		}
		if err := postJSON(ctx, client, endpoint, options.APIKey, input, &output); err != nil {
			return nil, err
		}
		if output.Results == nil {
			return nil, errors.New("missing results from Tavily")
		}
		results := make([]Result, 0, len(*output.Results))
		for _, hit := range *output.Results {
			results = append(results, Result{Title: hit.Title, URL: hit.URL, Snippet: hit.Content})
		}
		return results, nil
	}), nil
}

func newFirecrawl(_ context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		input := struct {
			Query   string   `json:"query"`
			Limit   int      `json:"limit"`
			Sources []string `json:"sources"`
		}{Query: query, Limit: ResultLimit, Sources: []string{"web"}}
		var output struct {
			Success bool `json:"success"`
			Data    *struct {
				Web []struct {
					Title       string `json:"title"`
					URL         string `json:"url"`
					Description string `json:"description"`
				} `json:"web"`
			} `json:"data"`
		}
		if err := postJSON(ctx, client, endpoint, options.APIKey, input, &output); err != nil {
			return nil, err
		}
		if !output.Success || output.Data == nil {
			return nil, errors.New("Firecrawl rejected the search request")
		}
		results := make([]Result, 0, len(output.Data.Web))
		for _, hit := range output.Data.Web {
			results = append(results, Result{Title: hit.Title, URL: hit.URL, Snippet: hit.Description})
		}
		return results, nil
	}), nil
}
