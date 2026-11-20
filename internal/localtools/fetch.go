package localtools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const maxFetchBody = 5 << 20
const maxFetchText = 60 << 10

func fetchLocalAgentURL(ctx context.Context, client *http.Client, target string) (string, error) {
	if client == nil {
		client = localWebClient
	}
	// Keep the configured transport (including the user's proxy), but inspect
	// redirects ourselves so approval for one origin cannot authorize another.
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, localWebClient.Timeout)
	defer cancel()
	for redirects := 0; ; redirects++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return "", err
		}
		request.Header.Set("Accept", "text/html, text/plain, application/json;q=0.9, */*;q=0.1")
		response, err := copyClient.Do(request)
		if err != nil {
			return "", err
		}
		switch response.StatusCode {
		case 301, 302, 303, 307, 308:
			response.Body.Close()
			if redirects >= 5 {
				return "", fmt.Errorf("重定向超过 5 次")
			}
			next, err := response.Location()
			if err != nil {
				return "", fmt.Errorf("无效的重定向地址: %w", err)
			}
			if next.User != nil || (next.Scheme != "https" && next.Scheme != "http") {
				return "", fmt.Errorf("不支持的重定向地址")
			}
			if !sameFetchOrigin(request.URL, next) {
				return "", fmt.Errorf("重定向到其他站点，请对新地址重新发起请求并按当前权限模式批准：%s", next.Redacted())
			}
			target = next.String()
			continue
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return "", fmt.Errorf("HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, maxFetchBody+1))
		if err != nil {
			return "", err
		}
		truncated := len(data) > maxFetchBody
		if truncated {
			data = data[:maxFetchBody]
			for i := 0; i < utf8.UTFMax-1 && len(data) > 0 && !utf8.Valid(data); i++ {
				data = data[:len(data)-1]
			}
		}
		if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
			return "", fmt.Errorf("目标不是 UTF-8 文本")
		}
		text := string(data)
		if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
			text = readableHTML(text, request.URL)
		}
		if len(text) > maxFetchText {
			end := maxFetchText
			for !utf8.RuneStart(text[end]) {
				end--
			}
			text, truncated = text[:end], true
		}
		if truncated {
			text += "\n\n[内容已截断：响应读取上限为 5 MiB，正文返回上限为 60 KiB。需要更多内容时，请读取页面中的具体链接。]"
		}
		return text, nil
	}
}

func sameFetchOrigin(a, b *url.URL) bool {
	if strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host) {
		return true
	}
	// Permit the usual HTTP to HTTPS upgrade, never a downgrade or port change.
	return a.Scheme == "http" && b.Scheme == "https" && a.Port() == "" && b.Port() == "" && strings.EqualFold(a.Hostname(), b.Hostname())
}

func readableHTML(source string, base *url.URL) string {
	z := html.NewTokenizer(strings.NewReader(source))
	var out strings.Builder
	skip := 0
	var links []string
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(out.String())
		case html.StartTagToken:
			token := z.Token()
			switch token.Data {
			case "script", "style", "noscript", "template", "svg":
				skip++
			case "p", "div", "br", "li", "h1", "h2", "h3", "tr":
				if skip == 0 {
					out.WriteByte('\n')
				}
			case "a":
				if skip == 0 {
					link := ""
					for _, attr := range token.Attr {
						if attr.Key != "href" {
							continue
						}
						u, err := url.Parse(attr.Val)
						if err != nil {
							continue
						}
						u = base.ResolveReference(u)
						if (u.Scheme == "https" || u.Scheme == "http") && u.User == nil {
							link = u.String()
						}
					}
					links = append(links, link)
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "noscript", "template", "svg":
				if skip > 0 {
					skip--
				}
			case "a":
				if skip == 0 && len(links) > 0 {
					link := links[len(links)-1]
					links = links[:len(links)-1]
					if link != "" {
						out.WriteString("(" + link + ") ")
					}
				}
			}
		case html.TextToken:
			if skip == 0 {
				text := strings.Join(strings.Fields(string(z.Text())), " ")
				if text != "" {
					out.WriteString(text)
					out.WriteByte(' ')
				}
			}
		}
	}
}
