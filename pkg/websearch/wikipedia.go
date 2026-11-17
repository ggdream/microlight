package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var wikipediaHighlights = strings.NewReplacer(`<span class="searchmatch">`, "", "</span>", "")

func newWikipedia(_ context.Context, client *http.Client, endpoint string, _ Options) (Provider, error) {
	return searchFunc(func(ctx context.Context, query string) ([]Result, error) {
		params := url.Values{
			"action": {"query"}, "list": {"search"}, "format": {"json"},
			"srsearch": {query}, "srlimit": {strconv.Itoa(ResultLimit)}, "srnamespace": {"0"}, "srprop": {"snippet"},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Microlight (https://github.com/ggdream/microlight)")
		response, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		var data struct {
			Query *struct {
				Hits []struct {
					Title   string `json:"title"`
					PageID  int    `json:"pageid"`
					Snippet string `json:"snippet"`
				} `json:"search"`
			} `json:"query"`
			Error *json.RawMessage `json:"error"`
		}
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
			return nil, err
		}
		if data.Error != nil || data.Query == nil {
			return nil, errors.New("Wikipedia rejected the search request")
		}
		results := make([]Result, 0, len(data.Query.Hits))
		for _, hit := range data.Query.Hits {
			if hit.PageID > 0 {
				results = append(results, Result{
					Title:   hit.Title,
					URL:     "https://en.wikipedia.org/?curid=" + strconv.Itoa(hit.PageID),
					Snippet: html.UnescapeString(wikipediaHighlights.Replace(hit.Snippet)),
				})
			}
		}
		return results, nil
	}), nil
}
