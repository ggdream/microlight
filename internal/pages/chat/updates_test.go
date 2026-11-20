package chat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/proxy"
)

func TestReleaseInstallerMatchesPublishedPlatformAssets(t *testing.T) {
	for _, tc := range []struct{ target, extension string }{
		{"darwin-amd64", "dmg"}, {"darwin-arm64", "dmg"},
		{"windows-amd64", "exe"}, {"linux-amd64", "tar.gz"}, {"android-arm64", "apk"},
	} {
		want := releasePage + "/download/v1.2.3/microlight-v1.2.3-" + tc.target + "." + tc.extension
		if got := releaseInstallerURL("v1.2.3", tc.target); got != want {
			t.Errorf("%s: got %q, want %q", tc.target, got, want)
		}
	}
	for _, target := range []string{"linux-arm64", "windows-arm64", "unknown"} {
		if got := releaseInstallerURL("v1.2.3", target); got != "" {
			t.Errorf("unsupported target %s offers a speculative URL: %s", target, got)
		}
	}
	for _, version := range []string{"dev", "v1.2.3-dirty", "v1.2.3+dirty", "v1.2.3/extra"} {
		if got := releaseInstallerURL(version, "darwin-arm64"); got != "" {
			t.Errorf("invalid tag %q offers a download: %s", version, got)
		}
	}
}

func TestUpdateRequestsHonorCustomProxy(t *testing.T) {
	for _, tc := range []struct {
		name string
		host string
		run  func(context.Context, proxy.Config) error
	}{
		{"latest release", "github.com:443", func(ctx context.Context, config proxy.Config) error {
			_, err := latestRelease(ctx, config)
			return err
		}},
		{"signed manifest", "github.com:443", func(ctx context.Context, config proxy.Config) error {
			source, err := newReleaseSource(config)
			if err != nil {
				return err
			}
			return closeUpdateResponse(source.Manifest(ctx, false))
		}},
		{"signed download", "release-assets.githubusercontent.com:443", func(ctx context.Context, config proxy.Config) error {
			source, err := newReleaseSource(config)
			if err != nil {
				return err
			}
			return closeUpdateResponse(source.Open(ctx, "https://release-assets.githubusercontent.com/update.tar.gz"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 4)
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.Method + " " + r.Host
				// Refuse the tunnel so the test cannot hit external servers.
				w.WriteHeader(http.StatusBadGateway)
			}))
			t.Cleanup(gateway.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			if err := tc.run(ctx, proxy.Config{Mode: proxy.Custom, URL: gateway.URL}); err == nil {
				t.Fatal("the request succeeded even though the configured proxy rejected it")
			}
			select {
			case got := <-requests:
				if want := http.MethodConnect + " " + tc.host; got != want {
					t.Fatalf("proxy received %q, want %q", got, want)
				}
			default:
				t.Fatal("the configured proxy did not receive the update request")
			}
		})
	}
}

func closeUpdateResponse(body io.ReadCloser, err error) error {
	if body != nil {
		_ = body.Close()
	}
	return err
}

func TestNewerReleaseAndSharedFocusCheck(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"v0.12.0+dirty", "v0.13.0", true},
		{"v0.12.0-abcdef0+dirty", "v0.12.0", false},
		{"v0.12.0", "v0.11.9", false},
		{"dev", "v0.13.0", false},
		{"v0.12.0", "bad", false},
	} {
		if got := newerRelease(tc.current, tc.latest); got != tc.want {
			t.Errorf("%s -> %s: %v", tc.current, tc.latest, got)
		}
	}
	app := &chatApp{updateCheck: updateCheck{checking: true, latest: "v1.0.0", started: time.Now()}}
	app.checkForUpdatesQuietly()
	if app.updateCheck.manual || !app.updateCheck.checking || app.updateCheck.latest != "v1.0.0" {
		t.Fatal("focus check disturbed the in-flight request")
	}
	app.checkForUpdates()
	if !app.updateCheck.manual || !app.updateCheck.checking {
		t.Fatal("manual check should subscribe to the in-flight request")
	}
}

func TestUpdateDownloadIndicatorOnlyAppearsForAvailableRelease(t *testing.T) {
	app := &chatApp{}
	window := ui.NewTester(app.updateDownloadButton, 300, 50)
	label := locales.DownloadInstaller() + " · v1.2.3"
	if _, ok := window.Find(label); ok {
		t.Fatal("download indicator should be hidden before finding a newer release")
	}
	app.updateCheck.latest = "v1.2.3"
	window = ui.NewTester(app.updateDownloadButton, 300, 50)
	button, ok := window.Find(label)
	if !ok || button.W != button.H {
		t.Fatal("available release needs a circular download action")
	}
}
