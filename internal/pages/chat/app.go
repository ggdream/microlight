package chat

import (
	"io/fs"
	"log"
	"os"

	"github.com/ggdream/microlight/internal/desktop"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/pkg/markup"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func Run(assets fs.FS) {
	var err error
	icons, err = asseticon.Load(assets)
	if err != nil {
		log.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	app := &chatApp{md: markup.New(ink), skillsHome: home}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err == nil {
		mygo.App.SetPath(mygo.PathUserData, dir)
		err = storage.MigrateAppDatabase(dir)
		if err == nil {
			app.store, err = openLocalStore(dir)
		}
	}
	app.loadLanguage()
	app.loadBusySendMode()
	if dir != "" {
		mygo.App.SetName(locales.AppName())
	}
	if err != nil {
		log.Printf("打开本地数据库失败：%v", err)
		app.historyErr = locales.HistoryOpenFailed()
		app.settingsLoadErr = locales.SettingsOpenFailed()
	} else {
		defer app.store.Close()
	}
	app.loadHistory()
	app.restoreDraft(app.history.Selected)
	app.loadSettings()
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title: locales.AppName(), Width: 1120, Height: 760,
			MinWidth: 660, MinHeight: 540, Content: ui.View(app.view),
		})
		mygo.App.OnLocaleChanged(func(tag string) { app.win.SetTitle(locales.In(tag).AppName()) })
		app.win.OnClosed(func() {
			app.persistDraft()
			app.clearConnectionTest()
			if app.running != nil {
				app.stopReply(app.active, app.running)
			}
		})
		app.win.OnShow(app.win.Invalidate)
		mygo.App.OnDidBecomeActive(app.win.Invalidate)
		app.win.OnFocus(func() {
			app.unreadReplies = 0
			mygo.App.SetBadgeCount(0)
			app.win.Invalidate()
		})
		desktop.SetupTray(app.win, assets)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
