package chat

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/material3"
)

func TestSettingsTransferBlocksCompactBackNavigation(t *testing.T) {
	for _, label := range []string{locales.ImportingSettings(), locales.ExportingSettings()} {
		app := &chatApp{settingsOpen: true, transferLoading: label, transferBusy: true}
		window := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(material3.Monochrome.Theme())
			app.settingsPage(c)
			app.settingsTransferFeedback(c.Key("feedback"))
		}, 390, 780)
		window.Key(0, ui.KeyEscape)
		if !app.settingsOpen || app.transferLoading == "" {
			t.Fatal("system Back should not dismiss settings or loading during transfer")
		}
	}
}

func TestSettingsImportResultUsesExpiringToast(t *testing.T) {
	for _, failed := range []bool{false, true} {
		app := &chatApp{}
		var err error
		if failed {
			err = errors.New("test failure")
		}
		app.settingsImportResult(err)
		want := locales.SettingsImported()
		if failed {
			want = locales.ImportSettingsFailed()
		}
		window := ui.NewTester(app.settingsTransferFeedback, 400, 400)
		window.Advance(250 * time.Millisecond)
		if !window.HasText(want) {
			t.Fatal("import result should appear as toast")
		}
		window.Advance(5 * time.Second)
		if window.HasText(want) {
			t.Fatal("import toast did not expire")
		}
	}
}

func TestSettingsExportResult(t *testing.T) {
	for _, outcome := range []struct {
		name, path, title string
		err               error
	}{
		{name: "success", path: "microlight-config.settings", title: locales.SettingsExported()},
		{name: "failure", err: errors.New("write failed"), title: locales.ExportSettingsFailed()},
		{name: "canceled"},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			app := &chatApp{transferBusy: true, transferLoading: locales.ExportingSettings()}
			app.settingsExportResult(outcome.path, outcome.err)
			if app.transferBusy || app.transferLoading != "" {
				t.Fatal("export did not release its loading state")
			}
			if outcome.title == "" {
				if app.transferToast != nil {
					t.Fatal("canceled export must not show a toast")
				}
				return
			}
			window := ui.NewTester(app.settingsTransferFeedback, 400, 400)
			window.Advance(250 * time.Millisecond)
			if !window.HasText(outcome.title) {
				t.Fatal("export result toast is missing")
			}
			window.Advance(5 * time.Second)
			if window.HasText(outcome.title) {
				t.Fatal("export result toast did not expire")
			}
		})
	}
}
