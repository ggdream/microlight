package chat

import (
	"encoding/json"
	"log/slog"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/storage"
)

const desktopConfigKey = "desktop_preferences"

type desktopPreferences struct {
	Shortcut     string `json:"shortcut"`
	Notification int    `json:"notification"`
	HidePreview  bool   `json:"hide_preview"`
}

const (
	notifyAll = iota
	notifyAction
	notifyOff
)

func (app *chatApp) loadDesktopPreferences() {
	if app.store == nil {
		return
	}
	raw, err := app.store.getConfig(desktopConfigKey)
	if err == nil && raw != "" {
		var preferences desktopPreferences
		if err = json.Unmarshal([]byte(raw), &preferences); err == nil {
			app.desktop = preferences
		}
	}
	if err != nil {
		slog.Error("read desktop preferences", "error", err)
	}
}

func (app *chatApp) saveDesktopPreferences(preferences desktopPreferences) bool {
	raw, err := json.Marshal(preferences)
	if err == nil && app.store != nil {
		err = storage.PutConfig(app.store.DB, desktopConfigKey, string(raw))
		if err == nil {
			app.desktop = preferences
			return true
		}
	}
	app.form.generalStatus = settingsStatus{message: locales.SettingsSaveFailed(), failed: true}
	return false
}

func (app *chatApp) registerShortcut(keys string) (string, error) {
	if keys == "" {
		return "", nil
	}
	actual := keys
	err := mygo.GlobalShortcut.RegisterWith(keys, mygo.GlobalShortcutOptions{
		Description: locales.DesktopShortcut(),
		OnKeys: func(bound string) {
			actual = bound
			if app.registeredShortcut == keys {
				app.actualShortcut = bound
				app.win.Invalidate()
			}
		},
	}, app.showComposer)
	return actual, err
}

func (app *chatApp) startDesktopShortcut() {
	if keys := app.desktop.Shortcut; keys != "" {
		if actual, err := app.registerShortcut(keys); err != nil {
			app.shortcutError = true
		} else {
			app.registeredShortcut, app.actualShortcut = keys, actual
		}
	}
}

func (app *chatApp) applyDesktopShortcut(keys string) {
	old := app.registeredShortcut
	if keys != "" && old != "" && sameShortcut(keys, old) {
		keys = old
	}
	actual := app.actualShortcut
	if keys != old {
		var err error
		actual, err = app.registerShortcut(keys)
		if err != nil {
			app.shortcutError = true
			return
		}
	}
	preferences := app.desktop
	preferences.Shortcut = keys
	if !app.saveDesktopPreferences(preferences) {
		if keys != old && keys != "" {
			mygo.GlobalShortcut.Unregister(keys)
		}
		return
	}
	if old != "" && old != keys {
		mygo.GlobalShortcut.Unregister(old)
	}
	if old != keys {
		app.registeredShortcut, app.actualShortcut = keys, actual
	}
	app.shortcutError = false
	app.form.generalStatus = settingsStatus{}
}

func (app *chatApp) showComposer() {
	if app.shortcutRecording || app.win.IsDestroyed() {
		return
	}
	if app.win.IsMinimized() {
		app.win.Restore()
	}
	mygo.App.Show()
	app.win.Show()
	mygo.App.Focus()
	app.win.Focus()
	app.win.Update(func() {
		focus := func() {
			if app.settingsOpen {
				app.closeSettings()
			}
			if chat := app.selected(); chat.waitingReply() != nil {
				chat.list.ScrollToEnd()
				return
			}
			app.focusComposer = true
		}
		if !app.deferSettingsLeave(focus) {
			focus()
		}
	})
}

func (app *chatApp) desktopSettings(c *ui.Context) {
	app.shortcutSettings(c.Key("shortcut"))
	settings.Card(c.Key("notifications"), locales.DesktopNotifications(), func() {
		preferences := app.desktop
		options := []string{locales.NotifyAllReplies(), locales.NotifyActionOnly(), locales.NotifyOff()}
		mode := options[clampNotificationMode(preferences.Notification)]
		ui.Select(c.Key("mode"), &mode, options).Label(locales.DesktopNotifications()).OnChange(func() {
			for index, label := range options {
				if mode == label {
					preferences.Notification = index
					app.saveNotificationSettings(preferences)
					break
				}
			}
		})
		preview := !preferences.HidePreview
		ui.Toggle(c.Key("preview"), &preview, locales.NotifyShowPreview()).
			Disabled(preferences.Notification == notifyOff).OnChange(func() {
			preferences.HidePreview = !preview
			app.saveNotificationSettings(preferences)
		})
	})
}

func clampNotificationMode(mode int) int {
	return max(notifyAll, min(notifyOff, mode))
}

func (app *chatApp) saveNotificationSettings(preferences desktopPreferences) {
	if app.saveDesktopPreferences(preferences) {
		app.form.generalStatus = settingsStatus{}
	}
}
