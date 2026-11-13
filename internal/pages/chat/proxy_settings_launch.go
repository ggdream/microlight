package chat

import (
	"os/exec"
	"runtime"

	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
)

func (app *chatApp) openProxySettingsControl(c *ui.Context) {
	var settingsURL string
	var command []string
	switch runtime.GOOS {
	case "darwin":
		settingsURL = "x-apple.systempreferences:com.apple.Network-Settings.extension?Proxies"
	case "windows":
		settingsURL = "ms-settings:network-proxy"
	case "linux":
		command = desktop.DesktopSettingsCommand()
	}
	if settingsURL == "" && len(command) == 0 {
		return
	}
	ui.Row(c).Children(func() {
		settings.ActionButton(c, icons, locales.OpenProxySettings(), "open", false).OnClick(func() {
			app.form.proxyLaunchFailed = false
			if settingsURL != "" {
				c.OpenURL(settingsURL)
				return
			}
			win := app.win
			go func() {
				if err := exec.Command(command[0], command[1:]...).Run(); err != nil {
					win.Update(func() {
						if app.settingsOpen {
							app.form.proxyLaunchFailed = true
						}
					})
				}
			}()
		})
	})
	if app.form.proxyLaunchFailed {
		ui.Text(c, locales.OpenProxySettingsFailed()).FontSize(12).TextColor(errorFG)
	}
}
