// Package linkpreview reads the title and description of a web page.
package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/ggdream/microlight/pkg/proxy"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type Metadata struct {
	Title       string
	Description string
}

// URL accepts web links without embedded credentials and strips fragments,
// which are local to the browser and do not change page metadata.
func URL(raw string) (*url.URL, bool) {
	if len(raw) > 8192 {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, false
	}
	u.Fragment, u.RawFragment = "", ""
	return u, true
}

const maxHTML = 512 << 10

// Fetch uses a separate, anonymous client and applies the application's proxy
// policy. Every destination, including redirects, must resolve publicly.
func Fetch(ctx context.Context, raw string, config proxy.Config) (Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	transport, err := publicTransport(config)
	if err != nil {
		return Metadata{}, err
	}
	defer transport.CloseIdleConnections()
	return fetch(ctx, raw, transport, previewTargetCheck(config))
}

func fetch(ctx context.Context, raw string, transport http.RoundTripper, check func(context.Context, *url.URL) error) (Metadata, error) {
	res, err := fetchPublic(ctx, raw, "text/html, application/xhtml+xml", transport, check)
	if err != nil {
		return Metadata{}, err
	}
	defer res.Body.Close()
	contentType := res.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType != "" && mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return Metadata{}, errors.New("preview response is not HTML")
	}
	reader, err := charset.NewReader(io.LimitReader(res.Body, maxHTML), contentType)
	if err != nil {
		return Metadata{}, err
	}
	return parseHead(reader), nil
}

func publicTransport(config proxy.Config) (*proxy.Transport, error) {
	transport, err := proxy.NewTransport(config)
	if err != nil {
		return nil, err
	}
	transport.RestrictDirectDial(publicDial)
	return transport, nil
}

func previewTargetCheck(config proxy.Config) func(context.Context, *url.URL) error {
	if runtime.GOOS == "js" {
		return func(ctx context.Context, target *url.URL) error {
			if _, ok := URL(target.String()); !ok {
				return errors.New("invalid preview URL")
			}
			host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
			if _, err := netip.ParseAddr(host); err == nil {
				return checkPublic(ctx, target)
			}
			if !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
				return errors.New("preview destination is local")
			}
			// Browsers own DNS, redirects and private-network/CORS policy. Go
			// cannot perform a separate DNS lookup or install a socket dialer.
			return nil
		}
	}
	if config.EffectiveMode() == proxy.Direct {
		return checkPublic
	}
	return func(ctx context.Context, target *url.URL) error {
		if _, ok := URL(target.String()); !ok {
			return errors.New("invalid preview redirect")
		}
		// Fake DNS can supply synthetic addresses for proxied domains.
		// A direct route still uses publicDial, which rejects these addresses.
		_, err := publicAddresses(ctx, target.Hostname(), true)
		return err
	}
}

func fetchPublic(ctx context.Context, raw, accept string, transport http.RoundTripper, check func(context.Context, *url.URL) error) (*http.Response, error) {
	target, ok := URL(raw)
	if !ok {
		return nil, errors.New("invalid preview URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := check(ctx, req.URL); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many preview redirects")
			}
			req.Header.Del("Referer")
			return check(req.Context(), req.URL)
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, fmt.Errorf("preview HTTP status %d", res.StatusCode)
	}
	return res, nil
}

func parseHead(reader io.Reader) Metadata {
	tokenizer := html.NewTokenizer(reader)
	var title strings.Builder
	var description, ogTitle, ogDescription string
	inTitle := false
	result := func() Metadata {
		return Metadata{Title: compact(prefer(ogTitle, title.String())), Description: compact(prefer(ogDescription, description))}
	}
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return result()
		case html.TextToken:
			if inTitle {
				title.WriteString(tokenizer.Token().Data)
			}
		case html.EndTagToken:
			switch tokenizer.Token().Data {
			case "title":
				inTitle = false
			case "head":
				return result()
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			switch token.Data {
			case "body":
				return result()
			case "title":
				inTitle = true
			case "meta":
				var key, content string
				for _, attr := range token.Attr {
					switch attr.Key {
					case "property", "name":
						if key == "" {
							key = strings.ToLower(strings.TrimSpace(attr.Val))
						}
					case "content":
						content = attr.Val
					}
				}
				switch key {
				case "og:title":
					ogTitle = prefer(ogTitle, content)
				case "og:description":
					ogDescription = prefer(ogDescription, content)
				case "description":
					description = prefer(description, content)
				}
			}
		}
	}
}

func prefer(primary, fallback string) string {
	if strings.TrimSpace(primary) == "" {
		return fallback
	}
	return primary
}

func compact(text string) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) > 420 {
		return string(runes[:419]) + "…"
	}
	return string(runes)
}

var nonPublicRoutes = [...]netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2001::/32"),
}

var syntheticDNSRange = netip.MustParsePrefix("198.18.0.0/15")

func publicAddresses(ctx context.Context, host string, allowSyntheticDNS bool) ([]netip.Addr, error) {
	name := strings.TrimSuffix(strings.ToLower(host), ".")
	_, literalErr := netip.ParseAddr(name)
	if literalErr != nil &&
		(!strings.Contains(name, ".") || strings.HasSuffix(name, ".localhost") || strings.HasSuffix(name, ".local")) {
		return nil, errors.New("preview destination is local")
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("preview destination has no addresses")
	}
	for _, ip := range ips {
		ip = ip.Unmap()
		if syntheticDNSRange.Contains(ip) && (!allowSyntheticDNS || literalErr == nil) {
			return nil, errors.New("preview destination is local")
		}
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.Zone() != "" {
			return nil, errors.New("preview destination is local")
		}
		for _, prefix := range nonPublicRoutes {
			if prefix.Contains(ip) {
				return nil, errors.New("preview destination is local")
			}
		}
	}
	return ips, nil
}

func checkPublic(ctx context.Context, target *url.URL) error {
	if _, ok := URL(target.String()); !ok {
		return errors.New("invalid preview redirect")
	}
	_, err := publicAddresses(ctx, target.Hostname(), false)
	return err
}

func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := publicAddresses(ctx, host, false)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 2 * time.Second}
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		err = dialErr
	}
	return nil, err
}
