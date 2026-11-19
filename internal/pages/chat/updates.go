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
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/buildinfo"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/pkg/proxy"
)

const releasePage = "https://github.com/ggdream/microlight/releases"

// The release script sets this for multi-architecture packages.
var updateTarget = runtime.GOOS + "-" + runtime.GOARCH

type updateCheck struct {
	checking bool
}

func (app *chatApp) versionSettings(c *ui.Context) {
	check := app.updateCheck
	version := buildinfo.Version()
	row := settings.AboutRow(c, ui.Column(c)).AlignItems(ui.Stretch).PaddingY(5)
	row.Children(func() {
		ui.Row(c).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(12).Children(func() {
			ui.Text(c, locales.AppVersion()).FontSize(viewport.Font(c, 15)).FontWeight(660)
			ui.Row(c).AlignItems(ui.Center).Gap(12).Shrink(0).Children(func() {
				ui.Text(c, version).FontSize(viewport.Font(c, 13)).TextColor(muted).Selectable()
				if desktop.Mobile {
					return
				}
				label := locales.CheckForUpdates()
				color := accent
				if check.checking {
					label = locales.CheckingForUpdates()
					color = muted
				}
				button := pointerButton(c.Key("check"), check.checking).Label(label).
					Height(33).PaddingX(6).Radius(6).AlignItems(ui.Center).Gap(5).
					FocusRingColor(accent)
				if !check.checking && button.Hovered() {
					button.Background(tint)
				}
				button.Children(func() {
					icon(c, "retry", color, 14)
					ui.Text(c, label).FontSize(viewport.Font(c, 13)).TextColor(color)
				}).OnClick(app.checkForUpdates)
			})
		})
	})
}

func (app *chatApp) checkForUpdates() {
	if app.updateCheck.checking {
		return
	}
	proxyConfig := app.settings.Proxy
	if mygo.Updater.Enabled() {
		source, err := newReleaseSource(proxyConfig)
		if err != nil {
			go app.showUpdateResult("", err)
			return
		}
		mygo.Updater.SetSource(source)
		updater.CheckForUpdates()
		return
	}
	app.updateCheck = updateCheck{checking: true}
	go func() {
		ctx, cancel := context.WithTimeout(app.appContext(), 12*time.Second)
		defer cancel()
		version, err := latestRelease(ctx, proxyConfig)
		if app.appContext().Err() == nil {
			app.showUpdateResult(version, err)
			app.win.Update(func() { app.updateCheck = updateCheck{} })
		}
	}()
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
	result, err := mygo.Dialog.Message(options)
	if err == nil && len(options.Buttons) == 2 && result.Button == 1 {
		_ = mygo.Shell.OpenExternal(releasePage + "/tag/" + version)
	}
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
