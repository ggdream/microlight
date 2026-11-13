package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
)

func TestMacSystemResolver(t *testing.T) {
	// Exercise CFNetwork without changing this computer's system settings.
	p, err := macProxy()
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("https://api.example.test/")
	if _, err := systemRoutes(t.Context(), target); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		_, _ = w.Write([]byte(`function FindProxyForURL(url, host) {
			return host == "api.example.test" ? "PROXY 127.0.0.1:4321; DIRECT" : "DIRECT";
		}`))
	}))
	defer server.Close()
	makeURL := func(raw string) uintptr {
		value := p.makeString(0, raw, cfUTF8)
		defer p.release(value)
		uri := p.makeURL(0, value, 0)
		if uri == 0 {
			t.Fatal("CFURLCreateWithString failed")
		}
		t.Cleanup(func() { p.release(uri) })
		return uri
	}
	got, err := p.resolvePAC(t.Context(), p.executeURL, makeURL(server.URL+"/proxy.pac"), makeURL(target.String()))
	if err != nil || !slices.Equal(got, []string{"http://127.0.0.1:4321", "direct://"}) {
		t.Fatalf("PAC routes = %v, error = %v", got, err)
	}
}
