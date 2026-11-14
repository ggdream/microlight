package chat

import (
	"testing"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestFullAccessDialogCountdown(t *testing.T) {
	app := &chatApp{draftAccess: localtools.Ask}
	surface := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() { app.accessControl(c, nil) })
		app.fullAccessDialog(c)
	}, 600, 400)
	click := func(label string) {
		t.Helper()
		if err := surface.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	open := func() {
		t.Helper()
		click(locales.ModeAsk())
		click(locales.ModeFull())
		if _, shown := surface.Find(locales.FullAccessTitle()); !shown {
			t.Fatal("choosing full access did not open the dialog")
		}
	}
	open()
	click(locales.ConfirmFullAccessCountdown(3))
	if app.draftAccess != localtools.Ask || app.fullAccess == nil {
		t.Fatal("the countdown button must not grant full access")
	}
	for seconds := 2; seconds >= 1; seconds-- {
		surface.Advance(time.Second)
		if _, shown := surface.Find(locales.ConfirmFullAccessCountdown(seconds)); !shown {
			t.Fatalf("the dialog did not show %d seconds remaining", seconds)
		}
	}
	surface.Advance(time.Second)
	click(locales.Cancel())
	if app.draftAccess != localtools.Ask || app.fullAccess != nil {
		t.Fatal("cancelling must leave permissions unchanged")
	}
	open()
	surface.Advance(3 * time.Second)
	if app.draftAccess != localtools.Ask {
		t.Fatal("the countdown must not grant access on its own")
	}
	click(locales.ConfirmFullAccess())
	if app.draftAccess != localtools.Full || app.fullAccess != nil {
		t.Fatal("confirmation after the countdown must grant full access and dismiss the dialog")
	}
}

func TestWorkspaceMenuAndSSHFormFollowLocale(t *testing.T) {
	originalLocale := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return originalLocale }) })
	for _, tc := range []struct {
		tag, local, docker, remote, title, host, auth, connect string
	}{
		{"zh-Hans", "本机目录", "本机容器", "远程主机", "配置远程工作区", "主机", "身份验证", "连接"},
		{"en", "Local", "Container", "Remote", "SSH Workspace", "Host", "Authentication", "Connect"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return tc.tag })
			app := &chatApp{}
			surface := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Justify(ui.End).Children(func() {
					app.workspaceControl(c.Key("workspace"), nil)
				})
				app.workspaceDialog(c.Key("dialog"))
			}, 580, 420)
			surface.SetLocale(tc.tag)
			if err := surface.Click(locales.In(tc.tag).Workspace()); err != nil {
				t.Fatal(err)
			}
			messages := locales.In(tc.tag)
			for _, label := range []string{tc.local, tc.docker, tc.remote, messages.WorkspaceBrowser(), messages.WorkspaceDesktop(), messages.WorkspaceMicroVM()} {
				if _, found := surface.Find(label); !found {
					t.Errorf("missing workspace menu item %q", label)
				}
			}
			if err := surface.Click(tc.remote); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{tc.title, tc.host, tc.auth, tc.connect} {
				if _, found := surface.Find(label); !found {
					t.Errorf("missing SSH form item %q", label)
				}
			}
			if err := surface.Click(locales.In(tc.tag).Cancel()); err != nil {
				t.Fatal(err)
			}
			if app.workspaceForm != nil {
				t.Fatal("cancel did not dismiss the workspace dialog")
			}
			if err := surface.Click(messages.Workspace()); err != nil {
				t.Fatal(err)
			}
			if err := surface.Click(messages.WorkspaceBrowser()); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{messages.WorkspaceBrowserEndpoint(), messages.WorkspaceBrowserToken(), tc.connect} {
				if _, found := surface.Find(label); !found {
					t.Errorf("missing browser form item %q", label)
				}
			}
			if err := surface.Click(messages.Cancel()); err != nil {
				t.Fatal(err)
			}
			if err := surface.Click(messages.Workspace()); err != nil {
				t.Fatal(err)
			}
			if err := surface.Click(messages.WorkspaceDesktop()); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{messages.WorkspaceDesktopEndpoint(), messages.WorkspaceDesktopToken(), tc.connect} {
				if _, found := surface.Find(label); !found {
					t.Errorf("missing desktop form item %q", label)
				}
			}
			if err := surface.Click(messages.Cancel()); err != nil {
				t.Fatal(err)
			}
			if err := surface.Click(messages.Workspace()); err != nil {
				t.Fatal(err)
			}
			if err := surface.Click(messages.WorkspaceMicroVM()); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{messages.WorkspaceMicroVMTitle(), tc.host, tc.connect} {
				if _, found := surface.Find(label); !found {
					t.Errorf("missing MicroVM form item %q", label)
				}
			}
		})
	}
}
