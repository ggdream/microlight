package chat

import (
	"context"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/background"
	"github.com/ggdream/microlight/internal/desktop"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/pkg/markup"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	updatenative "github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"
)

func Run(assets fs.FS) {
	home, _ := os.UserHomeDir()
	app := &chatApp{skillsHome: home}
	defer func() {
		if app.previewDirectory != "" {
			os.RemoveAll(app.previewDirectory)
		}
	}()
	var err error
	var dir string
	if desktop.Web {
		app.store, err = openLocalStore("")
	} else {
		var parent string
		parent, err = mygo.App.Path(mygo.PathAppData)
		if err == nil {
			dir, err = storage.PrepareAppDataDir(parent)
		}
		if err == nil {
			mygo.App.SetPath(mygo.PathUserData, dir)
			mygo.App.SetPath(mygo.PathLogs, filepath.Join(dir, "logs"))
			closeLogs, logErr := openAppLogs(dir)
			if logErr != nil {
				slog.Error("打开日志文件失败", "error", logErr)
			} else {
				defer closeLogs()
			}
			err = storage.MigrateAppDatabase(dir)
			if err == nil {
				app.store, err = openLocalStore(dir)
				if desktop.Mobile && err == nil {
					workspace := filepath.Join(dir, "workspace")
					if mkdirErr := os.MkdirAll(workspace, 0o700); mkdirErr != nil {
						slog.Error("initialize mobile workspace", "error", mkdirErr)
					} else {
						app.mobileWorkspace = workspace
					}
				}
			}
		}
	}
	if app.store != nil {
		defer app.store.Close()
	}
	if desktop.Desktop {
		mygo.Use(updatenative.New(updater.Options{DisableAutomaticChecks: true}))
	}
	loadedIcons, iconErr := asseticon.Load(assets)
	if iconErr != nil {
		slog.Error("读取应用图标失败", "error", iconErr)
		return
	}
	icons = loadedIcons
	brandImage, iconErr := fs.ReadFile(assets, "assets/icon.png")
	if iconErr == nil {
		app.brandIcon, iconErr = ui.DecodeBitmap(brandImage)
	}
	if iconErr != nil {
		slog.Error("读取应用图标失败", "error", iconErr)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	app.ctx = ctx
	if desktop.Desktop {
		tasks, taskErr := background.New(ctx)
		if taskErr != nil {
			slog.Error("initialize background tasks", "error", taskErr)
		} else {
			app.tasks = tasks
			defer func() {
				shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				if err := tasks.Close(shutdown); err != nil {
					slog.Error("close background tasks", "error", err)
				}
			}()
		}
	}
	defer cancel()
	offQuit := mygo.App.OnQuit(cancel)
	defer offQuit()
	app.md = markup.New(ink)
	app.configureLinkPreviews()
	app.loadLanguage()
	app.loadBusySendMode()
	app.loadSystemPrompt()
	app.loadDesktopPreferences()
	mygo.App.SetName(locales.AppName())
	if err != nil {
		slog.Error("打开本地数据库失败", "error", err)
		app.historyErr = locales.HistoryOpenFailed()
		app.settingsLoadErr = locales.SettingsOpenFailed()
	}
	app.loadHistory()
	app.restoreDraft(app.history.Selected)
	app.loadSettings()
	app.setMarkdownImageLoader(app.settings.Proxy)
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title: locales.AppName(), Width: 1120, Height: 760,
			MinWidth: 320, MinHeight: 360, Content: ui.View(app.view),
		})
		if desktop.Mobile {
			mygo.SystemBars.SetStyle(mygo.BarStyleDark)
		}
		mygo.App.OnLocaleChanged(func(tag string) { app.win.SetTitle(locales.In(tag).AppName()) })
		app.win.OnClosed(func() {
			app.persistDraft()
			app.clearConnectionTest()
			for _, chat := range app.history.Chats {
				if chat.run != nil {
					app.stopReply(chat, chat.run.reply)
				}
			}
		})
		app.win.OnShow(app.win.Invalidate)
		mygo.App.OnDidBecomeActive(app.win.Invalidate)
		app.win.OnFocus(func() {
			app.unreadReplies = 0
			mygo.App.SetBadgeCount(0)
			app.win.Invalidate()
		})
		if desktop.Desktop {
			desktop.SetupTray(app.win, assets)
			app.startDesktopShortcut()
		}
	})
	if err := mygo.App.Run(); err != nil {
		slog.Error("运行应用失败", "error", err)
	}
}

func (app *chatApp) appContext() context.Context {
	if app.ctx != nil {
		return app.ctx
	}
	return context.Background()
}

func openAppLogs(dir string) (func(), error) {
	dir = filepath.Join(dir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	appLog, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	errorLog, err := os.OpenFile(filepath.Join(dir, "error.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		appLog.Close()
		return nil, err
	}
	previous, writer, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewMultiHandler(
		slog.NewTextHandler(io.MultiWriter(writer, appLog), nil),
		slog.NewTextHandler(errorLog, &slog.HandlerOptions{Level: slog.LevelError}),
	)))
	return func() {
		slog.SetDefault(previous)
		log.SetOutput(writer)
		log.SetFlags(flags)
		appLog.Close()
		errorLog.Close()
	}, nil
}
