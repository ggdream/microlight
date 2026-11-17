package websearch

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/option"
)

func newGoogle(ctx context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	baseURL := strings.TrimSuffix(endpoint, "customsearch/v1")
	service, err := customsearch.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(baseURL))
	if err != nil {
		return nil, err
	}
	key, engineID := strings.TrimSpace(options.APIKey), strings.TrimSpace(options.SearchEngineID)
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		request := service.Cse.List().Context(ctx).Cx(engineID).Q(query).Num(ResultLimit)
		request.Header().Set("X-Goog-Api-Key", key)
		response, err := request.Do()
		if err != nil {
			return nil, err
		}
		results := make([]Result, 0, len(response.Items))
		for _, hit := range response.Items {
			if hit != nil {
				results = append(results, Result{Title: hit.Title, URL: hit.Link, Snippet: hit.Snippet})
			}
		}
		return results, nil
	}), nil
}
