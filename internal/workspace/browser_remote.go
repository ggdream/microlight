package workspace

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/chromedp/chromedp/remote"
	"github.com/zalando/go-keyring"
)

type remoteBrowserRuntime struct {
	*LocalBrowser
	sessionID string
}

func (b *remoteBrowserRuntime) Browser() BrowserDriver   { return b }
func (b *remoteBrowserRuntime) BrowserSessionID() string { return b.sessionID }
func (b *remoteBrowserRuntime) Environment() Environment { return Environment{} }
func (b *remoteBrowserRuntime) Close() error             { b.LocalBrowser.Close(); return nil }

func connectBrowser(ctx context.Context, target Target, secret string) (Runtime, error) {
	endpoint, err := url.Parse(target.Resource)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.ForceQuery || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("enter a browser WebSocket URL without credentials or query parameters")
	}
	session, ok := strings.CutPrefix(endpoint.Path, "/devtools/browser/")
	if !ok || session == "" || strings.Contains(session, "/") {
		return nil, errors.New("enter the full /devtools/browser/ WebSocket URL")
	}
	host := endpoint.Hostname()
	isLoopback := net.ParseIP(host).IsLoopback() || strings.EqualFold(host, "localhost")
	if endpoint.Scheme != "wss" && !(endpoint.Scheme == "ws" && isLoopback) {
		return nil, errors.New("use wss:// for a remote browser or ws:// through a loopback tunnel")
	}
	if secret == "" && target.Credential != "" {
		secret, err = keyring.Get(keyringService, target.Credential)
		if err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return nil, err
		}
	}
	headers := make(http.Header)
	if secret != "" {
		headers.Set("Authorization", "Bearer "+secret)
	}
	driver := &remoteBrowserRuntime{sessionID: session, LocalBrowser: &LocalBrowser{
		allocator: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return remote.NewAllocator(ctx, endpoint.String(), remote.NoModifyURL, remote.WithDialHTTPHeader(headers))
		},
	}}
	if _, err := driver.ListTabs(ctx); err != nil {
		driver.Close()
		return nil, err
	}
	return driver, nil
}
