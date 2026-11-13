// Package proxy provides HTTP transports for direct, custom, and system proxies.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
)

var ErrProxy = errors.New("proxy request failed")

type Mode string

const (
	System Mode = "system"
	Direct Mode = "direct"
	Custom Mode = "custom"
)

type Config struct {
	Mode Mode   `json:"mode"`
	URL  string `json:"url,omitempty"`
}

func (c Config) EffectiveMode() Mode {
	if c.Mode == "" {
		return System
	}
	return c.Mode
}

func parseURL(value string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Hostname() == "" || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("invalid proxy address")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u, nil
	default:
		return nil, errors.New("unsupported proxy protocol")
	}
}

type resolver func(context.Context, *url.URL) ([]*url.URL, error)
type proxyKey struct{}

type Transport struct {
	base    *http.Transport
	resolve resolver
}

func NewTransport(config Config) (*Transport, error) {
	switch config.EffectiveMode() {
	case System:
		return newTransport(systemRoutes), nil
	case Direct:
		return newTransport(nil), nil
	case Custom:
		u, err := parseURL(config.URL)
		if err != nil {
			return nil, err
		}
		return newTransport(func(context.Context, *url.URL) ([]*url.URL, error) {
			return []*url.URL{u}, nil
		}), nil
	default:
		return nil, errors.New("invalid proxy mode")
	}
}

func newTransport(resolve resolver) *Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = func(req *http.Request) (*url.URL, error) {
		u, _ := req.Context().Value(proxyKey{}).(*url.URL)
		return u, nil
	}
	return &Transport{base: base, resolve: resolve}
}

func (t *Transport) CloseIdleConnections() { t.base.CloseIdleConnections() }

func bypassLocal(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.SplitN(host, "%", 2)[0])
	return ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.resolve == nil || bypassLocal(req.URL.Hostname()) {
		return t.base.RoundTrip(req)
	}
	// Give PAC the origin for HTTPS, and keep credentials out of the resolver.
	target := *req.URL
	target.User, target.Fragment, target.RawFragment = nil, "", ""
	if target.Scheme == "https" {
		target.Path, target.RawPath, target.RawQuery, target.ForceQuery = "/", "", "", false
	}
	routes, err := t.resolve(req.Context(), &target)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("%w: %w", ErrProxy, err)
	}
	if len(routes) == 0 {
		return t.base.RoundTrip(req)
	}
	for i, route := range routes {
		var connected atomic.Bool
		ctx := context.WithValue(req.Context(), proxyKey{}, route)
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			GotConn: func(httptrace.GotConnInfo) { connected.Store(true) },
		})
		attempt := req.WithContext(ctx)
		if i != 0 && req.Body != nil && req.Body != http.NoBody {
			attempt.Body, err = req.GetBody()
			if err != nil {
				return nil, err
			}
		}
		response, err := t.base.RoundTrip(attempt)
		if route != nil && response != nil && response.StatusCode == http.StatusProxyAuthRequired {
			_ = response.Body.Close()
			return nil, ErrProxy
		}
		// PAC can provide backup routes. Retry only before a usable connection
		// is established, when the origin cannot have received the request.
		if err == nil || i == len(routes)-1 || connected.Load() || req.Context().Err() != nil ||
			(req.Body != nil && req.Body != http.NoBody && req.GetBody == nil) {
			if route != nil && err != nil && !connected.Load() {
				err = fmt.Errorf("%w: %w", ErrProxy, err)
			}
			return response, err
		}
	}
	return nil, ErrProxy
}

func parseRoutes(values []string) ([]*url.URL, error) {
	var routes []*url.URL
	for _, value := range values {
		if strings.EqualFold(value, "direct://") || strings.EqualFold(value, "DIRECT") {
			routes = append(routes, nil)
		} else if route, err := parseURL(value); err == nil {
			routes = append(routes, route)
		}
	}
	if len(values) != 0 && len(routes) == 0 {
		return nil, errors.New("system returned no supported proxy routes")
	}
	return routes, nil
}
