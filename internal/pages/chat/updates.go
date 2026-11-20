package chat

import (
	"context"
	"errors"
	"fmt"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/pkg/viewport"
	"io"
	"net/http"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/buildinfo"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/proxy"
)

const releasePage = "https://github.com/ggdream/microlight/releases"

// The release script sets this for multi-architecture packages.
var updateTarget = runtime.GOOS + "-" + runtime.GOARCH

type updateCheck struct {
	checking bool
	latest   string
	manual   bool
	started  time.Time
}

func (app *chatApp) versionSettings(c *ui.Context) {
	check := app.updateCheck
	version := buildinfo.Version()
	row := settings.AboutRow(c, ui.Column(c)).AlignItems(ui.Stretch).Justify(ui.Center).PaddingY(5)
	row.Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(12).Children(func() {
			ui.Text(c, locales.AppVersion()).FontSize(viewport.Font(c, 15)).FontWeight(660)
			ui.Row(c).AlignItems(ui.Center).Gap(12).Shrink(0).Children(func() {
				ui.Text(c, version).FontSize(viewport.Font(c, 13)).TextColor(muted).Selectable()
				if !desktop.Desktop {
					return
				}
				label := locales.CheckForUpdates()
				if check.checking {
					label = locales.CheckingForUpdates()
				}
				layout := windowLayout(c)
				color := material3.Foreground(material3.FilledButton, check.checking)
				button := material3.Button(c.Key("check"), label, material3.FilledButton, check.checking).
					Height(layout.Target(32)).PaddingX(layout.X(12)).Gap(layout.X(6))
				button.Children(func() {
					icon(c, "retry", color, 14)
					ui.Text(c, label).FontSize(viewport.Font(c, 13)).TextColor(color)
				}).OnClick(app.checkForUpdates)
			})
		})
	})
}

func (app *chatApp) checkForUpdates() {
	app.startUpdateCheck(true)
}

func (app *chatApp) checkForUpdatesQuietly() {
	if desktop.Web || time.Since(app.updateCheck.started) < time.Second {
		return
	}
	app.startUpdateCheck(false)
}

func (app *chatApp) startUpdateCheck(manual bool) {
	if app.updateCheck.checking {
		app.updateCheck.manual = app.updateCheck.manual || manual
		return
	}
	proxyConfig := app.settings.Proxy
	app.updateCheck.checking, app.updateCheck.manual, app.updateCheck.started = true, manual, time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(app.appContext(), 12*time.Second)
		defer cancel()
		version, err := latestRelease(ctx, proxyConfig)
		if app.appContext().Err() == nil {
			app.win.Update(func() {
				showResult := app.updateCheck.manual
				app.updateCheck.checking, app.updateCheck.manual = false, false
				if err == nil {
					app.updateCheck.latest = ""
					if newerRelease(buildinfo.Version(), version) {
						app.updateCheck.latest = version
					}
				}
				if showResult {
					go app.showUpdateResult(version, err)
				}
			})
		}
	}()
}

func newerRelease(current, latest string) bool {
	c, l := releaseParts(current), releaseParts(latest)
	return c != nil && l != nil && slices.Compare(l[:], c[:]) > 0
}

func (app *chatApp) updateDownloadButton(c *ui.Context) {
	version := app.updateCheck.latest
	if version == "" || desktop.Web {
		return
	}
	link := releaseInstallerURL(version, updateTarget)
	if link == "" {
		link = releasePage + "/tag/" + version
	}
	label := locales.DownloadInstaller() + " · " + version
	button := pointerButton(c.Key("update-download"), false).Label(label).Tooltip(label).
		Size(32, 32).Shrink(0).Radius(16).Background(ink).Center()
	button.Children(func() { icon(c, "download", white, 18) }).OnClick(func() { _ = mygo.Shell.OpenExternal(link) })
}

func (app *chatApp) showUpdateResult(version string, checkErr error) {
	options := mygo.MessageOptions{
		Parent: app.win, Title: locales.CheckForUpdates(), Type: mygo.MessageInfo,
	}
	if checkErr != nil {
		options.Type = mygo.MessageWarning
		options.Message = locales.UpdateCheckFailed()
	} else if current := releaseParts(buildinfo.Version()); current == nil {
		options.Message = locales.LatestRelease(version)
		options.Buttons = []string{locales.CloseChatInfo(), locales.ViewRelease()}
	} else if latest := releaseParts(version); latest != nil && slices.Compare(latest[:], current[:]) > 0 {
		options.Message = locales.UpdateAvailable(version)
		options.Buttons = []string{locales.CloseChatInfo(), locales.ViewRelease()}
	} else {
		options.Message = locales.AppUpToDate()
	}
	downloadURL := releaseInstallerURL(version, runtime.GOOS+"-"+runtime.GOARCH)
	if len(options.Buttons) == 2 && downloadURL != "" {
		options.Buttons = append(options.Buttons, locales.DownloadInstaller())
	}
	result, err := app.showMessage(options)
	if err == nil && len(options.Buttons) >= 2 {
		switch result.Button {
		case 1:
			_ = mygo.Shell.OpenExternal(releasePage + "/tag/" + version)
		case 2:
			if downloadURL != "" {
				_ = mygo.Shell.OpenExternal(downloadURL)
			}
		}
	}
}

// Match the public installer names produced by package-release.sh. Unsupported
// targets still offer the release page instead of a speculative download URL.
func releaseInstallerURL(version, target string) string {
	parts := releaseParts(version)
	if parts == nil || version != fmt.Sprintf("v%d.%d.%d", parts[0], parts[1], parts[2]) {
		return ""
	}
	var extension string
	switch target {
	case "darwin-amd64", "darwin-arm64":
		extension = "dmg"
	case "windows-amd64":
		extension = "exe"
	case "linux-amd64":
		extension = "tar.gz"
	case "android-arm64":
		extension = "apk"
	}
	if extension == "" {
		return ""
	}
	return releasePage + "/download/" + version + "/microlight-" + version + "-" + target + "." + extension
}

type releaseSource struct {
	client *http.Client
}

func newReleaseSource(config proxy.Config) (*releaseSource, error) {
	transport, err := proxy.NewTransport(config)
	if err != nil {
		return nil, err
	}
	return &releaseSource{client: &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) >= 10 {
				return errors.New("invalid update redirect")
			}
			return nil
		},
	}}, nil
}

func (s *releaseSource) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	// Microlight publishes only stable releases.
	client := *s.client
	client.Timeout = 30 * time.Second
	return downloadReleaseFile(ctx, &client, releasePage+"/latest/download/update-"+updateTarget+".json")
}

func (s *releaseSource) Open(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	return downloadReleaseFile(ctx, s.client, rawURL)
}

func downloadReleaseFile(ctx context.Context, client *http.Client, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" || req.URL.Host == "" || req.URL.User != nil {
		return nil, errors.New("update files require an HTTPS URL")
	}
	req.Header.Set("User-Agent", "Microlight-Updater")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("update file request returned HTTP %d", res.StatusCode)
	}
	return res.Body, nil
}

func latestRelease(ctx context.Context, config proxy.Config) (string, error) {
	transport, err := proxy.NewTransport(config)
	if err != nil {
		return "", err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasePage+"/latest", nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	location, err := res.Location()
	if err != nil || res.StatusCode < 300 || res.StatusCode >= 400 || location.Scheme != "https" ||
		location.Host != "github.com" || path.Dir(location.Path) != "/ggdream/microlight/releases/tag" {
		return "", errors.New("unexpected release response")
	}
	version := path.Base(location.Path)
	if releaseParts(version) == nil || strings.ContainsAny(version, "-+") {
		return "", errors.New("unexpected release tag")
	}
	return version, nil
}

func releaseParts(version string) *[3]int {
	version = strings.TrimSuffix(version, "+dirty")
	version, _, _ = strings.Cut(version, "-")
	if !strings.HasPrefix(version, "v") {
		return nil
	}
	var parts [3]int
	if _, err := fmt.Sscanf(version, "v%d.%d.%d", &parts[0], &parts[1], &parts[2]); err != nil ||
		parts[0] < 0 || parts[1] < 0 || parts[2] < 0 ||
		version != fmt.Sprintf("v%d.%d.%d", parts[0], parts[1], parts[2]) {
		return nil
	}
	return &parts
}
