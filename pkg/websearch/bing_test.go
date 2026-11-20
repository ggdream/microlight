package websearch

import (
	"encoding/base64"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func bingResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestBingSearchNormalizesOrganicResults(t *testing.T) {
	calls := 0
	provider, endpoint, err := NewProvider(t.Context(), BingCN, Options{}, &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" || req.URL.Host != "cn.bing.com" || req.URL.Query().Get("q") != "微光 Go" || req.URL.Query().Get("count") != "5" || req.URL.Query().Get("setlang") != "zh-hans" || req.Header.Get("User-Agent") == "" || req.Header.Get("Accept-Language") == "" {
			t.Fatalf("invalid search request: %s", req.URL)
		}
		return bingResponse(`<li class="b_ad"><h2><a href="https://ad.example/">Ad</a></h2></li>
   <li class="b_algo"><h2><a href="https://example.com/doc">Example <b>title</b></a></h2><div class="b_caption"><p> A  snippet </p></div></li>
   <li class="b_algo"><h2><a href="https://example.com/doc">Duplicate</a></h2></li>
   <li class="b_algo"><h2><a href="javascript:alert(1)">Invalid</a></h2></li>`), nil
	})})
	if err != nil || calls != 0 || endpoint != "https://cn.bing.com/search" {
		t.Fatalf("initialization: %s %v calls=%d", endpoint, err, calls)
	}
	hits, err := provider.Search(t.Context(), "微光 Go")
	want := []Result{{Title: "Example title", URL: "https://example.com/doc", Snippet: "A snippet"}}
	if err != nil || calls != 1 || !reflect.DeepEqual(hits, want) {
		t.Fatalf("hits=%+v err=%v calls=%d", hits, err, calls)
	}
}

func TestBingResultURLResolutionDoesNotFetchDestinations(t *testing.T) {
	for _, encoded := range []bool{false, true} {
		calls := 0
		href := "https://cn.bing.com/ck/a?token=opaque"
		if encoded {
			href = "https://cn.bing.com/ck/a?u=a1" + base64.RawURLEncoding.EncodeToString([]byte("https://example.com/doc"))
		}
		provider, _, _ := NewProvider(t.Context(), BingCN, Options{}, &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			if req.URL.Hostname() == "example.com" {
				t.Fatal("search must not fetch result destinations")
			}
			if req.URL.Path == "/ck/a" {
				resp := bingResponse("")
				resp.StatusCode = 302
				resp.Header.Set("Location", "https://example.com/doc")
				return resp, nil
			}
			return bingResponse(`<li class="b_algo"><h2><a href="` + href + `">Example</a></h2></li>`), nil
		})})
		hits, err := provider.Search(t.Context(), "Go")
		if err != nil || len(hits) != 1 || hits[0].URL != "https://example.com/doc" {
			t.Fatalf("hits=%+v err=%v", hits, err)
		}
		wantCalls := 2
		if encoded {
			wantCalls = 1
		}
		if calls != wantCalls {
			t.Fatalf("calls=%d want=%d", calls, wantCalls)
		}
	}
}

func TestBingDistinguishesEmptyResultsFromFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, kind string
		status           int
	}{
		{"empty", `<body><div class="b_no">No matches</div></body>`, "", 200},
		{"challenge", `<title>访问验证</title><body><form action="/captcha"></form></body>`, "verification_required", 200},
		{"layout changed", `<html><body>Unexpected page</body></html>`, "invalid_response", 200},
		{"rate limited", `<html>Too many requests</html>`, "rate_limited", 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, _, _ := NewProvider(t.Context(), BingCN, Options{}, &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				resp := bingResponse(tc.body)
				resp.StatusCode = tc.status
				return resp, nil
			})})
			hits, err := provider.Search(t.Context(), "query")
			if tc.kind == "" {
				if err != nil || len(hits) != 0 {
					t.Fatalf("empty results: %+v %v", hits, err)
				}
				return
			}
			if err == nil || DescribeFailure(err).Kind != tc.kind {
				t.Fatalf("want %s got %v", tc.kind, err)
			}
		})
	}
}

func TestBingRedirectPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, location string
		wantCalls      int
		wantError      bool
	}{
		{"same engine", "https://www.bing.com/results", 2, false},
		{"external search redirect", "https://example.com/", 1, true},
		{"redirect loop", "https://cn.bing.com/search", maxSearchRedirects + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			provider, _, _ := NewProvider(t.Context(), BingCN, Options{}, &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Hostname() == "example.com" {
					t.Fatal("unexpected external request")
				}
				if req.URL.Path == "/results" {
					return bingResponse(`<li class="b_algo"><h2><a href="https://example.com/">Example</a></h2></li>`), nil
				}
				resp := bingResponse("")
				resp.StatusCode = 302
				resp.Header.Set("Location", tc.location)
				return resp, nil
			})})
			_, err := provider.Search(t.Context(), "query")
			if (err != nil) != tc.wantError || calls != tc.wantCalls {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
