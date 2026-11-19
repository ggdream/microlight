package chat

import (
	"encoding/json"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/pkg/viewport"
	"log/slog"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/internal/theme"
)

const desktopConfigKey = "desktop_preferences"

type desktopPreferences struct {
	Shortcut       string `json:"shortcut"`
	Notification   int    `json:"notification"`
	HideStatistics bool   `json:"hide_statistics,omitempty"`
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
	if desktop.Mobile {
		return
	}
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
	if !desktop.Mobile {
		app.shortcutSettings(c.Key("shortcut"))
	}
	settings.Card(c.Key("notifications"), locales.DesktopNotifications(), func() {
		preferences := app.desktop
		options := []string{locales.NotifyAllReplies(), locales.NotifyActionOnly(), locales.NotifyOff()}
		mode := clampNotificationMode(preferences.Notification)
		layout := windowLayout(c)
		dropdown := ui.SelectBase(c.Key("mode"), &mode)
		trigger := dropdown.Trigger.Label(locales.DesktopNotifications()).Value(options[mode]).FillWidth().
			Height(settings.SelectHeight(c)).PaddingX(layout.X(10)).AlignItems(ui.Center).Justify(ui.SpaceBetween).
			Gap(layout.X(8)).Border(1, border).Radius(7).Background(canvas).Cursor(ui.CursorPointer).FocusRingColor(accent)
		if trigger.Hovered() {
			trigger.Background(side)
		}
		trigger.Children(func() {
			ui.Text(c, options[mode]).MinWidth(0).SingleLine().Ellipsis("…").FontSize(viewport.Font(c, 12)).TextColor(ink)
			ui.Icon(c, icons.SVG("ios_chevron_right")).Size(14, 14).Shrink(0).Rotate(90).TextColor(muted)
		}).OnChange(func() {
			preferences.Notification = mode
			app.saveNotificationSettings(preferences)
		})
		dropdown.Popup(func(panel ui.Element) {
			panel.Padding(layout.Y(4), layout.X(4)).Gap(layout.Y(2)).Background(white).Border(1, border).Radius(8)
			for index, label := range options {
				item := dropdown.Item(index).Label(label).Padding(layout.Y(6), layout.X(8)).Radius(5).
					AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(layout.X(8)).Cursor(ui.CursorPointer)
				if item.Highlighted() {
					item.Background(tint)
				}
				item.Children(func() {
					ui.Text(c, label).FontSize(viewport.Font(c, 12)).TextColor(ink)
					if index == mode {
						icon(c, "check", accent, 14)
					}
				})
			}
		})
		ui.Row(c.Key("test")).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
			button := settings.ActionButton(c.Key("send"), icons, locales.SendTestNotification(), "bell", app.testingNotification)
			if layout := windowLayout(c); layout.Compact() {
				button.MinHeight(max(34, layout.Y(34)))
			}
			button.OnClick(app.sendTestNotification)
			if status := app.notificationTestStatus; status.message != "" {
				color := theme.Muted
				if status.failed {
					color = theme.ErrorFG
				}
				ui.Text(c, status.message).Grow(1).MinWidth(0).FontSize(viewport.Font(c, 12)).TextColor(color)
			}
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
