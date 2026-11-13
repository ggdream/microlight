package desktop

import (
	"os/exec"
	"slices"
	"testing"
)

func TestLinuxProxySettingsLauncher(t *testing.T) {
	for _, tc := range []struct {
		desktop, session string
		installed        []string
		want             []string
	}{
		{"ubuntu:GNOME", "", []string{"gnome-control-center", "systemsettings"}, []string{"/usr/bin/gnome-control-center", "network", "configure-proxy"}},
		{"KDE", "", []string{"systemsettings", "gnome-control-center"}, []string{"/usr/bin/systemsettings", "kcm_proxy"}},
		{"", "plasmawayland", []string{"kcmshell5"}, []string{"/usr/bin/kcmshell5", "kcm_proxy"}},
		{"XFCE", "", []string{"gnome-control-center"}, nil},
		{"GNOME", "", []string{"systemsettings"}, nil},
	} {
		lookPath := func(name string) (string, error) {
			if slices.Contains(tc.installed, name) {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		}
		got := linuxSettingsCommand(tc.desktop, tc.session, lookPath)
		if !slices.Equal(got, tc.want) {
			t.Errorf("desktop %q, session %q: got %v, want %v", tc.desktop, tc.session, got, tc.want)
		}
	}
}
