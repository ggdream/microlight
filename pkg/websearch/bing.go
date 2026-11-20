package websearch

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

const searchUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
const maxSearchRedirects = 5

type bingSearch struct {
	client   *http.Client
	endpoint string
}

func newBingCN(_ context.Context, client *http.Client, endpoint string, _ Options) (Provider, error) {
	return &bingSearch{client: client, endpoint: endpoint}, nil
}

func isSearchRedirect(status int) bool {
	switch status {
	case 301, 302, 303, 307, 308:
		return true
	}
	return false
}

func (s *bingSearch) Search(ctx context.Context, query string) ([]Result, error) {
	endpoint, err := url.Parse(s.endpoint)
	if err != nil {
		return nil, err
	}
	params := endpoint.Query()
	params.Set("q", query)
	params.Set("count", strconv.Itoa(ResultLimit))
	params.Set("setlang", "zh-hans")
	endpoint.RawQuery = params.Encode()
	page, body, contentType, err := s.page(ctx, endpoint, false)
	if err != nil {
		return nil, err
	}
	reader, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, err
	}
	title := strings.ToLower(doc.Find("title").Text())
	if strings.Contains(page.Path, "/challenge") || doc.Find("#b_captcha, #captcha, form[action*='captcha'], iframe[src*='captcha']").Length() > 0 || strings.Contains(title, "验证码") || strings.Contains(title, "人机验证") || strings.Contains(title, "访问验证") || strings.Contains(title, "captcha") {
		return nil, &Failure{Kind: "verification_required", Message: "Bing requires browser verification before it can return search results.", Detail: "Open Bing in a browser or choose another search provider.", Retryable: false}
	}
	results := make([]Result, 0, ResultLimit)
	seen := make(map[string]bool)
	// Organic result cards only; ads and unrelated page navigation are excluded.
	doc.Find("li.b_algo").EachWithBreak(func(_ int, card *goquery.Selection) bool {
		anchor := card.Find("h2 a[href]").First()
		title := strings.Join(strings.Fields(anchor.Text()), " ")
		raw, _ := anchor.Attr("href")
		link := absoluteSearchURL(page, raw)
		if title == "" || link == nil {
			return true
		}
		link = s.resultURL(ctx, link)
		if seen[link.String()] {
			return true
		}
		snippet := strings.Join(strings.Fields(card.Find(".b_caption p").First().Text()), " ")
		results = append(results, Result{Title: title, URL: link.String(), Snippet: snippet})
		seen[link.String()] = true
		return len(results) < ResultLimit
	})
	if len(results) == 0 {
		text := doc.Find("body").Text()
		if doc.Find(".b_no").Length() == 0 && !strings.Contains(text, "没有与此相关的结果") && !strings.Contains(text, "There are no results for") {
			return nil, &Failure{Kind: "invalid_response", Message: "Bing returned a page without recognizable search results.", Detail: "The page may be a verification screen or Bing's HTML layout may have changed.", Retryable: true}
		}
	}
	return results, nil
}

func absoluteSearchURL(base *url.URL, raw string) *url.URL {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	parsed = base.ResolveReference(parsed)
	if parsed.User != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil
	}
	return parsed
}

func bingHost(u *url.URL) bool {
	if u.Port() != "" && u.Port() != "443" && u.Port() != "80" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "cn.bing.com", "www.bing.com", "bing.com":
		return true
	}
	return false
}

// Search redirects stay on Bing hosts. When resolving a result's jump link,
// return an external destination without fetching that destination.
func (s *bingSearch) page(ctx context.Context, u *url.URL, resolving bool) (*url.URL, []byte, string, error) {
	for hop := 0; hop <= maxSearchRedirects; hop++ {
		if !bingHost(u) {
			return nil, nil, "", &Failure{Kind: "invalid_response", Message: "Bing redirected to an unexpected host."}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, nil, "", err
		}
		req.Header.Set("User-Agent", searchUserAgent)
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, nil, "", err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSearchResponse+1))
		resp.Body.Close()
		if readErr != nil {
			return nil, nil, "", readErr
		}
		if len(body) > maxSearchResponse {
			return nil, nil, "", &Failure{Kind: "invalid_response", Message: "The search response is too large."}
		}
		if isSearchRedirect(resp.StatusCode) {
			next := absoluteSearchURL(u, resp.Header.Get("Location"))
			if next == nil {
				return nil, nil, "", &Failure{Kind: "invalid_response", Message: "Bing returned an invalid redirect."}
			}
			if !bingHost(next) {
				if resolving {
					return next, nil, "", nil
				}
				return nil, nil, "", &Failure{Kind: "invalid_response", Message: "Bing redirected to an unexpected host."}
			}
			u = next
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, nil, "", responseFailure(BingCN, resp.StatusCode, upstreamError{}, "")
		}
		return u, body, resp.Header.Get("Content-Type"), nil
	}
	return nil, nil, "", &Failure{Kind: "invalid_response", Message: fmt.Sprintf("Bing exceeded %d redirects.", maxSearchRedirects)}
}

func (s *bingSearch) resultURL(ctx context.Context, link *url.URL) *url.URL {
	if !bingHost(link) || !strings.HasPrefix(link.Path, "/ck/a") {
		return link
	}
	// Bing's destination parameter is URL-safe Base64 prefixed with "a1".
	encoded := strings.TrimPrefix(link.Query().Get("u"), "a1")
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err == nil {
		if target := absoluteSearchURL(link, string(decoded)); target != nil && !bingHost(target) {
			return target
		}
	}
	target, _, _, err := s.page(ctx, link, true)
	if err == nil && !bingHost(target) {
		return target
	}
	return link // Keep the usable jump link if best-effort resolution fails.
}
