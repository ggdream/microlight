package localtools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	maxWebBody    = 5 << 20
	webChunkRunes = 20_000
)

func webURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("需要有效且不含登录凭据的 HTTP(S) 地址")
	}
	u.Fragment, u.RawFragment = "", ""
	return u, nil
}

func sameWebSite(source, target *url.URL) bool {
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	if !strings.EqualFold(source.Hostname(), target.Hostname()) {
		return false
	}
	if source.Scheme == target.Scheme {
		return port(source) == port(target)
	}
	return source.Scheme == "http" && target.Scheme == "https" && port(source) == "80" && port(target) == "443"
}

type webPage struct {
	URL            string `json:"url"`
	Title          string `json:"title,omitempty"`
	Content        string `json:"content,omitempty"`
	NextOffset     int    `json:"next_offset,omitempty"`
	DownloadCapped bool   `json:"download_capped,omitempty"`
	RedirectURL    string `json:"redirect_url,omitempty"`
}

func fetchLocalAgentURL(ctx context.Context, client *http.Client, target string, offset int) (string, error) {
	if client == nil || client.Transport == nil {
		return "", errors.New("网页抓取未配置应用网络连接")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/html, application/xhtml+xml, text/plain, text/markdown, */*;q=0.1")
	reader := *client
	reader.Jar = nil
	var redirectURL string
	reader.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		destination, err := webURL(req.URL.String())
		if err != nil {
			return err
		}
		if !sameWebSite(via[0].URL, destination) || !sameWebSite(via[len(via)-1].URL, destination) {
			redirectURL = destination.String()
			return http.ErrUseLastResponse
		}
		if len(via) > 5 {
			return errors.New("网页重定向次数超过 5 次")
		}
		req.Header.Del("Referer")
		return nil
	}
	response, err := reader.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	page := webPage{URL: response.Request.URL.String(), RedirectURL: redirectURL}
	if redirectURL == "" {
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return "", fmt.Errorf("HTTP %d", response.StatusCode)
		}
		if err := page.read(response, offset); err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(page)
	return string(data), err
}

func (page *webPage) read(response *http.Response, offset int) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxWebBody+1))
	if err != nil {
		return err
	}
	page.DownloadCapped = len(body) > maxWebBody
	if page.DownloadCapped {
		end := maxWebBody
		for end > 0 && !utf8.RuneStart(body[end]) {
			end--
		}
		body = body[:end]
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}
	isHTML := mediaType == "text/html" || mediaType == "application/xhtml+xml"
	if !isHTML && !strings.HasPrefix(mediaType, "text/") && mediaType != "application/json" &&
		mediaType != "application/xml" && !strings.HasSuffix(mediaType, "+json") && !strings.HasSuffix(mediaType, "+xml") {
		return fmt.Errorf("不支持的网页内容类型 %s", mediaType)
	}
	var content string
	if isHTML {
		reader, err := charset.NewReader(bytes.NewReader(body), contentType)
		if err != nil {
			return err
		}
		page.Title, content, err = extractWebHTML(reader)
		if err != nil {
			return err
		}
	} else {
		if encoding, _ := charset.Lookup(params["charset"]); encoding != nil {
			body, err = encoding.NewDecoder().Bytes(body)
			if err != nil {
				return err
			}
		}
		content = strings.TrimSpace(strings.TrimPrefix(string(body), "\uFEFF"))
		if !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
			return errors.New("网页内容无法解码为文本")
		}
	}
	if content == "" {
		return errors.New("网页中没有可提取的正文")
	}
	if offset < 0 || offset >= len(content) || !utf8.RuneStart(content[offset]) {
		return errors.New("网页正文偏移量无效")
	}
	end := len(content)
	count := 0
	for index := range content[offset:] {
		if count == webChunkRunes {
			end = offset + index
			break
		}
		count++
	}
	if end < len(content) {
		if line := strings.LastIndexByte(content[offset:end], '\n'); line >= (end-offset)/2 {
			end = offset + line + 1
		}
		page.NextOffset = end
	}
	page.Content = content[offset:end]
	return nil
}

func extractWebHTML(reader io.Reader) (title, content string, err error) {
	doc, err := html.Parse(reader)
	if err != nil {
		return "", "", err
	}
	var body, main, article *html.Node
	for node := range doc.Descendants() {
		if node.Type != html.ElementNode {
			continue
		}
		switch node.Data {
		case "title":
			title = webText(node)
		case "body":
			body = node
		case "main":
			if main == nil {
				main = node
			}
		case "article":
			if article == nil {
				article = node
			}
		}
	}
	for _, node := range []*html.Node{main, article, body} {
		if node != nil {
			if content := webText(node); content != "" {
				return title, content, nil
			}
		}
	}
	return title, "", nil
}

func webText(root *html.Node) string {
	var output, line strings.Builder
	flush := func() {
		if text := strings.Join(strings.Fields(line.String()), " "); text != "" {
			if output.Len() > 0 {
				output.WriteByte('\n')
			}
			output.WriteString(text)
		}
		line.Reset()
	}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			line.WriteString(node.Data)
			return
		}
		if node.Type != html.ElementNode {
			return
		}
		for _, attr := range node.Attr {
			if attr.Key == "hidden" || (attr.Key == "aria-hidden" && attr.Val == "true") {
				return
			}
		}
		switch node.Data {
		case "script", "style", "template", "noscript", "svg", "nav", "footer", "aside":
			return
		case "pre":
			flush()
			if output.Len() > 0 {
				output.WriteByte('\n')
			}
			for child := range node.Descendants() {
				if child.Type == html.TextNode {
					output.WriteString(child.Data)
				}
			}
			return
		case "p", "div", "section", "header", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "br", "blockquote":
			flush()
			defer flush()
		case "td", "th":
			line.WriteByte(' ')
		}
		for child := range node.ChildNodes() {
			visit(child)
		}
	}
	visit(root)
	flush()
	return strings.TrimSpace(output.String())
}
