package desktop

import (
	"io/fs"
	"log"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo"
)

func SetupTray(window *mygo.Window, assets fs.FS) {
	appIcon, err := fs.ReadFile(assets, "assets/icon.png")
	if err != nil {
		log.Printf("读取应用图标失败：%v", err)
		return
	}
	trayIcon, err := fs.ReadFile(assets, "assets/tray.png")
	if err != nil {
		log.Printf("读取托盘图标失败：%v", err)
		return
	}
	if err := mygo.App.Dock.SetIcon(appIcon); err != nil {
		log.Printf("设置应用图标失败：%v", err)
	}
	menu := func() *mygo.Menu {
		return mygo.NewMenu([]*mygo.MenuItem{
			{Label: locales.ShowWindow(), Click: func(*mygo.MenuItem, *mygo.Window) { window.Show() }},
			{Type: mygo.MenuItemSeparator},
			{Label: locales.QuitApp(), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		})
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "microlight-chat",
		Icon:           trayIcon,
		IconIsTemplate: true,
		ToolTip:        locales.AppName(),
		Menu:           menu(),
	})
	if err != nil {
		log.Printf("创建系统托盘失败：%v", err)
		return
	}
	mygo.App.OnLocaleChanged(func(tag string) {
		tray.SetToolTip(locales.In(tag).AppName())
		tray.SetMenu(menu())
	})

	quitting := false
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
	window.OnClose(func(event *mygo.CloseEvent) {
		if !quitting {
			event.PreventDefault()
			window.Hide()
		}
	})
	mygo.App.OnActivate(func(visible bool) {
		if !visible && !quitting {
			window.Show()
		}
	})
	mygo.App.OnSecondInstance(func([]string, string) { window.Show() })
}
