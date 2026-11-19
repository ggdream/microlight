package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSystemResolutionFailureDoesNotBypassProxy(t *testing.T) {
	var reached atomic.Bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
	}))
	defer origin.Close()
	failed := errors.New("PAC unavailable")
	transport := newTransport(func(context.Context, *url.URL) ([]*url.URL, error) { return nil, failed })
	defer transport.CloseIdleConnections()
	// Use a non-local target so the deliberate loopback bypass does not apply.
	transport.RestrictDirectDial(func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(origin.URL, "http://"))
	})
	_, err := (&http.Client{Transport: transport}).Get("http://proxy-required.example/")
	if !errors.Is(err, ErrProxy) || !errors.Is(err, failed) || reached.Load() {
		t.Fatalf("proxy resolution failure must remain a proxy error, without reaching origin: %v", err)
	}
}

func TestSystemRoutesRefreshWithoutRecreatingTransport(t *testing.T) {
	server := func(value string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, value)
		}))
	}
	first, second := server("first"), server("second")
	defer first.Close()
	defer second.Close()
	current, _ := url.Parse(first.URL)
	transport := newTransport(func(context.Context, *url.URL) ([]*url.URL, error) { return []*url.URL{current}, nil })
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	for _, test := range []struct{ proxy, want string }{{first.URL, "first"}, {second.URL, "second"}} {
		current, _ = url.Parse(test.proxy)
		response, err := client.Get("http://proxy-required.example/")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(body) != test.want {
			t.Fatalf("request did not follow the updated system route: body=%q error=%v", body, err)
		}
	}
}
