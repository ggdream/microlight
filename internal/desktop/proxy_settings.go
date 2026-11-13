package desktop

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

var DesktopSettingsCommand = sync.OnceValue(func() []string {
	return linuxSettingsCommand(os.Getenv("XDG_CURRENT_DESKTOP"), os.Getenv("DESKTOP_SESSION"), exec.LookPath)
})

func linuxSettingsCommand(desktop, session string, lookPath func(string) (string, error)) []string {
	if desktop == "" {
		desktop = session
	}
	for _, name := range strings.Split(strings.ToLower(desktop), ":") {
		switch strings.TrimSpace(name) {
		case "gnome", "gnome-classic", "ubuntu", "ubuntu-wayland", "ubuntu-xorg":
			if path, err := lookPath("gnome-control-center"); err == nil {
				// Older releases open Network, which contains the proxy entry.
				return []string{path, "network", "configure-proxy"}
			}
			return nil
		case "kde", "plasma", "plasmawayland", "plasmax11":
			for _, executable := range []string{"systemsettings", "systemsettings5", "kcmshell6", "kcmshell5"} {
				if path, err := lookPath(executable); err == nil {
					return []string{path, "kcm_proxy"}
				}
			}
			return nil
		}
	}
	return nil
}
