package websearch

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ResultLimit = 5

var (
	ErrUnknownProvider = errors.New("unknown web search provider")
	ErrInvalidKey      = errors.New("search API key contains a line break")
	ErrMissingKey      = errors.New("search API key is required")
	ErrMissingEngineID = errors.New("Google Search Engine ID is required")
	ErrInvalidURL      = errors.New("invalid SearXNG base URL")
)

type ID string

const (
	DuckDuckGo ID = "duckduckgo"
	BingCN     ID = "bingcn"
	SearXNG    ID = "searxng"
	Google     ID = "google"
	Wikipedia  ID = "wikipedia"
	Tavily     ID = "tavily"
	Firecrawl  ID = "firecrawl"
	DeepSeek   ID = "deepseek"
	Baidu      ID = "baidu"
	Aliyun     ID = "aliyun"
	Tencent    ID = "tencent"
	Volcengine ID = "volcengine"
)

type Options struct {
	APIKey         string `json:"api_key,omitempty"`
	BaseURL        string `json:"base_url,omitempty"`
	SearchEngineID string `json:"search_engine_id,omitempty"`
}

type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type Provider interface {
	Search(context.Context, string) ([]Result, error)
}

type searchFunc func(context.Context, string) ([]Result, error)

func (search searchFunc) Search(ctx context.Context, query string) ([]Result, error) {
	return search(ctx, query)
}

type Definition struct {
	ID             ID
	Name           string
	APIKey         bool
	BaseURL        bool
	SearchEngineID bool
	endpoint       string
	newSearch      func(context.Context, *http.Client, string, Options) (Provider, error)
}

var definitions = []Definition{
	{ID: DuckDuckGo, Name: "DuckDuckGo", endpoint: "https://html.duckduckgo.com/html/", newSearch: newDuckDuckGo},
	{ID: BingCN, Name: "Bing China", endpoint: "https://cn.bing.com/search", newSearch: newBingCN},
	{ID: SearXNG, Name: "SearXNG", BaseURL: true, newSearch: newSearXNG},
	{ID: Google, Name: "Google Custom Search", APIKey: true, SearchEngineID: true, endpoint: "https://customsearch.googleapis.com/customsearch/v1", newSearch: newGoogle},
	{ID: Wikipedia, Name: "Wikipedia", endpoint: "https://en.wikipedia.org/w/api.php", newSearch: newWikipedia},
	{ID: Tavily, Name: "Tavily", APIKey: true, endpoint: "https://api.tavily.com/search", newSearch: newTavily},
	{ID: Firecrawl, Name: "Firecrawl", APIKey: true, endpoint: "https://api.firecrawl.dev/v2/search", newSearch: newFirecrawl},
	{ID: DeepSeek, Name: "DeepSeek", APIKey: true, endpoint: "https://api.deepseek.com/v1/tools/search", newSearch: newDeepSeek},
	{ID: Baidu, Name: "Baidu", APIKey: true, endpoint: "https://qianfan.baidubce.com/v2/ai_search/web_search", newSearch: newBaidu},
	{ID: Aliyun, Name: "Alibaba Cloud IQS", APIKey: true, endpoint: "https://cloud-iqs.aliyuncs.com/search/unified", newSearch: newAliyun},
	{ID: Tencent, Name: "Tencent Cloud Yuanbao Search", APIKey: true, endpoint: "https://api.wsa.cloud.tencent.com/SearchPro", newSearch: newTencent},
	{ID: Volcengine, Name: "Volcengine Doubao Search", APIKey: true, endpoint: "https://open.feedcoopapi.com/search_api/web_search", newSearch: newVolcengine},
}

func IDs() []ID {
	ids := make([]ID, 0, len(definitions))
	for _, definition := range definitions {
		ids = append(ids, definition.ID)
	}
	return ids
}

func Lookup(id ID) (Definition, bool) {
	for _, definition := range definitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

func (definition Definition) Validate(options Options, required bool) error {
	if strings.ContainsAny(options.APIKey, "\r\n") {
		return ErrInvalidKey
	}
	if definition.APIKey && required && strings.TrimSpace(options.APIKey) == "" {
		return ErrMissingKey
	}
	if definition.SearchEngineID && required && strings.TrimSpace(options.SearchEngineID) == "" {
		return ErrMissingEngineID
	}
	if definition.BaseURL && (required || options.BaseURL != "") && !validEndpoint(options.BaseURL) {
		return ErrInvalidURL
	}
	return nil
}

// NewProvider opens the selected search provider without making a network request.
// The returned endpoint is the destination for search queries.
func NewProvider(ctx context.Context, id ID, options Options, client *http.Client) (Provider, string, error) {
	definition, ok := Lookup(id)
	if !ok {
		return nil, "", ErrUnknownProvider
	}
	if err := definition.Validate(options, true); err != nil {
		return nil, "", err
	}
	endpoint := definition.endpoint
	if definition.BaseURL {
		endpoint = strings.TrimRight(options.BaseURL, "/") + "/search"
	}
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	bounded.Transport = searchTransport{RoundTripper: transport, id: id, credential: options.APIKey}
	provider, err := definition.newSearch(ctx, &bounded, endpoint, options)
	return provider, endpoint, err
}

func validEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") &&
		u.User == nil && !u.ForceQuery && u.RawQuery == "" && !strings.Contains(raw, "#")
}
