package websearch

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (transport testTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestProvidersNormalizeOfficialSearchResponses(t *testing.T) {
	const query = "微光"
	for _, test := range []struct {
		id       ID
		options  Options
		endpoint string
		request  string
		response string
	}{
		{
			id: DeepSeek, options: Options{APIKey: "test-key"},
			endpoint: "https://api.deepseek.com/v1/tools/search",
			request:  `{"query":"微光","count":5}`,
			response: `{"results":[{"title":"Example","url":"https://example.com/","summary":"Snippet"}]}`,
		},
		{
			id: Baidu, options: Options{APIKey: "test-key"},
			endpoint: "https://qianfan.baidubce.com/v2/ai_search/web_search",
			request:  `{"messages":[{"role":"user","content":"微光"}],"search_source":"baidu_search_v2","edition":"standard","resource_type_filter":[{"type":"web","top_k":5}]}`,
			response: `{"references":[{"title":"Example","url":"https://example.com/","snippet":"Snippet"}]}`,
		},
		{
			id: Aliyun, options: Options{APIKey: "test-key"},
			endpoint: "https://cloud-iqs.aliyuncs.com/search/unified",
			request:  `{"query":"微光","engineType":"CNAuto","advancedParams":{"numResults":5}}`,
			response: `{"pageItems":[{"title":"Example","link":"https://example.com/","snippet":"Snippet"}]}`,
		},
		{
			id: Tencent, options: Options{APIKey: "test-key"},
			endpoint: "https://api.wsa.cloud.tencent.com/SearchPro",
			request:  `{"Query":"微光"}`,
			response: `{"Response":{"Pages":["{\"title\":\"Example\",\"url\":\"https://example.com/\",\"passage\":\"Snippet\"}"]}}`,
		},
		{
			id: Volcengine, options: Options{APIKey: "test-key"},
			endpoint: "https://open.feedcoopapi.com/search_api/web_search",
			request:  `{"Query":"微光","SearchType":"web","Count":5,"Filter":{"NeedUrl":true}}`,
			response: `{"ResponseMetadata":{"RequestId":"test-request"},"Result":{"WebResults":[{"Title":"Example","Url":"https://example.com/","Snippet":"Short","Summary":"Snippet"}]}}`,
		},
		{
			id: SearXNG, options: Options{BaseURL: "http://searx.example.test"},
			endpoint: "http://searx.example.test/search",
			response: `{"query":"微光","results":[{"title":"Example","url":"https://example.com/","content":"Snippet"}]}`,
		},
		{
			id: Google, options: Options{APIKey: "test-key", SearchEngineID: "test-cx"},
			endpoint: "https://customsearch.googleapis.com/customsearch/v1",
			response: `{"items":[{"title":"Example","link":"https://example.com/","snippet":"Snippet"}]}`,
		},
		{
			id: Wikipedia, endpoint: "https://en.wikipedia.org/w/api.php",
			response: `{"query":{"search":[{"title":"Example","pageid":42,"snippet":"<span class=\"searchmatch\">Snippet</span>"}]}}`,
		},
		{
			id: Tavily, options: Options{APIKey: "test-key"},
			endpoint: "https://api.tavily.com/search",
			request:  `{"query":"微光","search_depth":"basic","max_results":5}`,
			response: `{"results":[{"title":"Example","url":"https://example.com/","content":"Snippet"}]}`,
		},
		{
			id: Firecrawl, options: Options{APIKey: "test-key"},
			endpoint: "https://api.firecrawl.dev/v2/search",
			request:  `{"query":"微光","limit":5,"sources":["web"]}`,
			response: `{"success":true,"data":{"web":[{"title":"Example","url":"https://example.com/","description":"Snippet"}]}}`,
		},
		{
			id: DuckDuckGo, endpoint: "https://html.duckduckgo.com/html/",
			response: `<div id="links"><div class="web-result">
				<h2 class="result__title"><a href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2F">Example</a></h2>
				<a class="result__snippet">Snippet</a></div></div>`,
		},
	} {
		t.Run(string(test.id), func(t *testing.T) {
			requests := 0
			transport := testTransport(func(request *http.Request) (*http.Response, error) {
				requests++
				if got := request.URL.Scheme + "://" + request.URL.Host + request.URL.Path; got != test.endpoint {
					t.Errorf("endpoint = %q, want %q", got, test.endpoint)
				}
				switch test.id {
				case Google:
					params := request.URL.Query()
					if request.Method != http.MethodGet || params.Get("cx") != "test-cx" || params.Get("q") != query ||
						params.Get("num") != "5" || request.Header.Get("X-Goog-Api-Key") != "test-key" {
						t.Error("invalid Google search request")
					}
				case Wikipedia:
					params := request.URL.Query()
					if request.Method != http.MethodGet || params.Get("list") != "search" ||
						params.Get("srsearch") != query || params.Get("srlimit") != "5" {
						t.Error("invalid Wikipedia search request")
					}
				case SearXNG:
					if request.Method != http.MethodGet || request.URL.Query().Get("q") != query || request.URL.Query().Get("format") != "json" {
						t.Error("invalid SearXNG search request")
					}
				case DuckDuckGo:
					if err := request.ParseForm(); err != nil || request.PostForm.Get("q") != query {
						t.Errorf("invalid DuckDuckGo form: %v", err)
					}
				default:
					var got, want any
					if e1, e2 := json.NewDecoder(request.Body).Decode(&got), json.Unmarshal([]byte(test.request), &want); e1 != nil || e2 != nil || !reflect.DeepEqual(got, want) {
						t.Errorf("request = %#v, want %#v", got, want)
					}
					if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer test-key" {
						t.Error("invalid method or authorization")
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.response))}, nil
			})
			provider, endpoint, err := NewProvider(t.Context(), test.id, test.options, &http.Client{Transport: transport})
			if err != nil || endpoint != test.endpoint || requests != 0 {
				t.Fatalf("provider init: endpoint %q, requests %d, error %v", endpoint, requests, err)
			}
			results, err := provider.Search(t.Context(), query)
			want := []Result{{Title: "Example", URL: "https://example.com/", Snippet: "Snippet"}}
			if test.id == Wikipedia {
				want[0].URL = "https://en.wikipedia.org/?curid=42"
			}
			if err != nil || !reflect.DeepEqual(results, want) || requests != 1 {
				t.Fatalf("search: results %+v, requests %d, error %v", results, requests, err)
			}
		})
	}
}
