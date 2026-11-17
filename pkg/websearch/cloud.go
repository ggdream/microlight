package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func newAliyun(_ context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		input := struct {
			Query    string `json:"query"`
			Engine   string `json:"engineType"`
			Advanced struct {
				Count int `json:"numResults"`
			} `json:"advancedParams"`
		}{Query: query, Engine: "CNAuto"}
		input.Advanced.Count = ResultLimit
		var output struct {
			Pages *[]struct {
				Title   string `json:"title"`
				Link    string `json:"link"`
				Snippet string `json:"snippet"`
			} `json:"pageItems"`
		}
		if err := postJSON(ctx, client, endpoint, options.APIKey, input, &output); err != nil {
			return nil, err
		}
		if output.Pages == nil {
			return nil, errors.New("missing pageItems from Alibaba Cloud IQS")
		}
		results := make([]Result, 0, len(*output.Pages))
		for _, page := range *output.Pages {
			results = append(results, Result{Title: page.Title, URL: page.Link, Snippet: page.Snippet})
		}
		return results, nil
	}), nil
}

func newTencent(_ context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		input := struct {
			Query string `json:"Query"`
		}{Query: query}
		var output struct {
			Response *struct {
				Error *json.RawMessage `json:"Error"`
				Pages []string         `json:"Pages"`
			} `json:"Response"`
		}
		if err := postJSON(ctx, client, endpoint, options.APIKey, input, &output); err != nil {
			return nil, err
		}
		if output.Response == nil || output.Response.Error != nil {
			return nil, errors.New("Tencent Cloud Yuanbao Search rejected the request")
		}
		// SearchPro encodes each page as a JSON string in the response array.
		results := make([]Result, 0, min(ResultLimit, len(output.Response.Pages)))
		for _, raw := range output.Response.Pages {
			var page struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Passage string `json:"passage"`
				Content string `json:"content"`
			}
			if json.Unmarshal([]byte(raw), &page) != nil || page.URL == "" {
				continue
			}
			snippet := page.Passage
			if strings.TrimSpace(snippet) == "" {
				snippet = page.Content
			}
			results = append(results, Result{Title: page.Title, URL: page.URL, Snippet: snippet})
			if len(results) == ResultLimit {
				break
			}
		}
		if len(output.Response.Pages) != 0 && len(results) == 0 {
			return nil, errors.New("invalid pages from Tencent Cloud Yuanbao Search")
		}
		return results, nil
	}), nil
}

func newVolcengine(_ context.Context, client *http.Client, endpoint string, options Options) (Provider, error) {
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		input := struct {
			Query  string `json:"Query"`
			Type   string `json:"SearchType"`
			Count  int    `json:"Count"`
			Filter struct {
				NeedURL bool `json:"NeedUrl"`
			} `json:"Filter"`
		}{Query: query, Type: "web", Count: ResultLimit}
		input.Filter.NeedURL = true
		var output struct {
			Metadata struct {
				Error *json.RawMessage `json:"Error"`
			} `json:"ResponseMetadata"`
			Result *struct {
				Pages []struct {
					Title   string `json:"Title"`
					URL     string `json:"Url"`
					Snippet string `json:"Snippet"`
					Summary string `json:"Summary"`
				} `json:"WebResults"`
			} `json:"Result"`
		}
		if err := postJSON(ctx, client, endpoint, options.APIKey, input, &output); err != nil {
			return nil, err
		}
		if output.Metadata.Error != nil || output.Result == nil {
			return nil, errors.New("Volcengine Doubao Search rejected the request")
		}
		results := make([]Result, 0, len(output.Result.Pages))
		for _, page := range output.Result.Pages {
			snippet := page.Summary
			if strings.TrimSpace(snippet) == "" {
				snippet = page.Snippet
			}
			results = append(results, Result{Title: page.Title, URL: page.URL, Snippet: snippet})
		}
		return results, nil
	}), nil
}
