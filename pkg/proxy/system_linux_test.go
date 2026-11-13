package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"testing"
)

func TestLinuxDesktopPAC(t *testing.T) {
	if os.Getenv("MICROLIGHT_TEST_GIO_PAC") != "1" {
		t.Skip("run in an isolated Linux desktop environment with glib-networking and GNOME schemas")
	}
	t.Setenv("GSETTINGS_BACKEND", "keyfile")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		_, _ = w.Write([]byte(`function FindProxyForURL(url, host) {
			return host == "api.example.test" ? "PROXY 127.0.0.1:4321" : "DIRECT";
		}`))
	}))
	defer server.Close()
	for _, setting := range [][2]string{{"autoconfig-url", server.URL + "/proxy.pac"}, {"mode", "auto"}} {
		cmd := exec.CommandContext(t.Context(), "gsettings", "set", "org.gnome.system.proxy", setting[0], setting[1])
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gsettings failed: %s (%v)", output, err)
		}
	}
	for _, test := range []struct {
		host string
		want []string
	}{
		{"api.example.test", []string{"http://127.0.0.1:4321"}},
		{"direct.example.test", []string{"direct://"}},
	} {
		target := &url.URL{Scheme: "https", Host: test.host, Path: "/"}
		routes, err := systemRoutes(t.Context(), target)
		var values []string
		for _, route := range routes {
			if route == nil {
				values = append(values, "direct://")
			} else {
				values = append(values, route.String())
			}
		}
		if err != nil || !slices.Equal(values, test.want) {
			t.Errorf("PAC routes for %s = %v, error = %v, want %v", test.host, values, err, test.want)
		}
	}
}
