package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
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

type updateCheck struct {
	checking bool
	version  string
	failed   bool
}

func (app *chatApp) updateSettings(c *ui.Context) {
	check := app.updateCheck
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(12).
		Background(white).Radius(13).Border(1, border).Padding(12, 16).Children(func() {
		ui.Column(c).Grow(1).MinWidth(140).Gap(4).Children(func() {
			ui.Text(c, locales.AppUpdates()).FontSize(15).FontWeight(660)
			switch {
			case check.failed:
				ui.Text(c, locales.UpdateCheckFailed()).FontSize(12).TextColor(errorFG)
			case check.version != "":
				label := locales.AppUpToDate()
				color := muted
				if current, latest := releaseParts(buildinfo.Version()), releaseParts(check.version); current == nil {
					label = locales.LatestRelease(check.version)
				} else if slices.Compare(latest[:], current[:]) > 0 {
					label = locales.UpdateAvailable(check.version)
					color = accent
				}
				ui.Text(c, label).FontSize(12).TextColor(color)

				release := pointerButton(c.Key("release"), false).Label(locales.ViewRelease()).
					AlignSelf(ui.Start).AlignItems(ui.Center).Gap(5).Height(24).Radius(4).
					FocusRingColor(accent)
				color = muted
				if release.Hovered() {
					color = accent
				}
				release.Children(func() {
					ui.Text(c, locales.ViewRelease()).FontSize(12).TextColor(color)
					icon(c, "open", color, 12)
				}).OnClick(func() { c.OpenURL(releasePage + "/tag/" + check.version) })
			}
		})
		label := locales.CheckForUpdates()
		if check.checking {
			label = locales.CheckingForUpdates()
		}
		settings.ActionButton(c.Key("check"), icons, label, "retry", check.checking).
			FocusRingColor(accent).OnClick(app.checkForUpdates)
	})
}

func (app *chatApp) checkForUpdates() {
	if app.updateCheck.checking {
		return
	}
	if mygo.Updater.Enabled() {
		updater.CheckForUpdates()
		return
	}
	app.updateCheck = updateCheck{checking: true}
	proxyConfig := app.settings.Proxy
	go func() {
		ctx, cancel := context.WithTimeout(app.appContext(), 12*time.Second)
		defer cancel()
		version, err := latestRelease(ctx, proxyConfig)
		if app.appContext().Err() == nil {
			app.win.Update(func() {
				app.updateCheck = updateCheck{version: version, failed: err != nil}
			})
		}
	}()
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
	if releaseParts(version) == nil || strings.Contains(version, "-") {
		return "", errors.New("unexpected release tag")
	}
	return version, nil
}

func releaseParts(version string) *[3]int {
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
