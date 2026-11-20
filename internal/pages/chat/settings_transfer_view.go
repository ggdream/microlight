package chat

import (
	"bytes"
	"encoding/json"
	"github.com/ggdream/microlight/pkg/viewport"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/pkg/material3"
)

const settingsFilename = "microlight-config.settings"

func settingsFileFilters() []mygo.FileFilter {
	return []mygo.FileFilter{
		{Name: locales.SettingsFileFilter(), Extensions: []string{"settings"}},
		{Name: locales.AllFiles(), Extensions: []string{"*"}},
	}
}

func (app *chatApp) settingsTransfer(c *ui.Context) {
	layout := windowLayout(c)
	settings.Card(c, locales.SettingsTransfer(), func() {
		ui.Text(c, locales.SettingsTransferHint()).FontSize(viewport.Font(c, 13)).TextColor(muted)
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(layout.X(8)).Children(func() {
			disabled := app.transferBusy || app.store == nil
			action := func(key, label, glyph string, click func()) {
				button := settings.ActionButton(c.Key(key), icons, label, glyph, disabled)
				if layout.Compact() {
					button.Grow(1).Basis(0).MinWidth(0).Justify(ui.Center)
				}
				button.OnClick(click)
			}
			action("import", locales.ImportSettings(), "folder_open", app.chooseSettingsImport)
			action("export", locales.ExportSettings(), "download", app.chooseSettingsExport)
		})
	})
}

func (app *chatApp) chooseSettingsExport() {
	if app.transferBusy || app.win == nil || app.store == nil {
		return
	}
	app.transferBusy, app.transferLoading = true, locales.ExportingSettings()
	app.transferToast = nil
	// Capture the saved settings on the UI thread. Encoding and saving run in
	// the background, while the shared modal HUD blocks further interaction.
	file := app.settingsExportFile()
	win := app.win
	title, filters := locales.ExportSettings(), settingsFileFilters()[:1]
	work := func() {
		data, err := json.MarshalIndent(file, "", "  ")
		var path string
		if err == nil {
			dir, _ := mygo.App.Path(mygo.PathDownloads)
			path, err = desktop.SaveDocument(mygo.SaveDialogOptions{
				Parent: win, Title: title, DefaultPath: filepath.Join(dir, settingsFilename),
				Filters: filters,
			}, append(data, '\n'))
		}
		win.Update(func() {
			app.settingsExportResult(path, err)
		})
	}
	if !app.deferBrowserAction(work) {
		go work()
	}
}

func (app *chatApp) chooseSettingsImport() {
	if app.transferBusy || app.win == nil || app.store == nil {
		return
	}
	app.transferBusy, app.transferToast = true, nil
	win := app.win
	go func() {
		file, config, err := app.chooseSettingsFile(win)
		win.Update(func() {
			if app.appContext().Err() != nil {
				app.transferBusy = false
				return
			}
			switch {
			case err != nil:
				app.transferBusy = false
				app.settingsImportResult(err)
			case file != nil:
				app.startSettingsImport(*file, config)
			default:
				app.transferBusy = false
			}
		})
	}()
}

func (app *chatApp) settingsTransferFeedback(c *ui.Context) {
	if app.transferToast != nil {
		c.AddToast(*app.transferToast)
		app.transferToast = nil
	}
	if app.transferLoading != "" {
		material3.LoadingOverlay(c, app.transferLoading)
	}
}

func (app *chatApp) settingsImportResult(err error) {
	toast := ui.Toast{ID: "settings-import", Title: locales.SettingsImported(), Type: "success"}
	if err != nil {
		toast.Title, toast.Type = locales.ImportSettingsFailed(), "error"
	}
	app.transferToast = &toast
}

func (app *chatApp) settingsExportResult(path string, err error) {
	app.transferBusy, app.transferLoading = false, ""
	app.transferToast = nil
	if err == nil && path == "" {
		return
	}
	toast := ui.Toast{ID: "settings-export", Title: locales.SettingsExported(), Type: "success"}
	if err != nil {
		toast.Title, toast.Type = locales.ExportSettingsFailed(), "error"
	}
	app.transferToast = &toast
}

func (app *chatApp) startSettingsImport(file settingsFile, config modelSettings) {
	if app.transferLoading != "" || app.win == nil || app.store == nil {
		return
	}
	app.transferLoading, app.transferBusy = locales.ImportingSettings(), true
	win, store := app.win, app.store
	work := func() {
		backend, err := prepareSettingsImport(store, file, config)
		if app.appContext().Err() != nil {
			if backend != nil && backend.network != nil {
				backend.network.CloseIdleConnections()
			}
			return
		}
		win.Update(func() {
			if err == nil {
				app.installSettingsImport(file, config, backend)
			}
			app.transferLoading, app.transferBusy = "", false
			app.settingsImportResult(err)
		})
	}
	// Browser SQLite work must run outside the DOM callback, using the same
	// asynchronous queue as the other configuration operations.
	if !app.deferBrowserAction(work) {
		go work()
	}
}

func (app *chatApp) chooseSettingsFile(win *mygo.Window) (*settingsFile, modelSettings, error) {
	var source io.ReadCloser
	if desktop.Web {
		files, err := desktop.OpenBrowserFiles(false, ".settings", 8<<20)
		if err != nil || len(files) == 0 {
			return nil, modelSettings{}, err
		}
		source = io.NopCloser(bytes.NewReader(files[0].Data))
	} else {
		paths, err := desktop.OpenFilteredFiles(mygo.OpenDialogOptions{
			Parent: win, Title: locales.ImportSettings(),
			Filters: settingsFileFilters(),
		})
		if err != nil || len(paths) == 0 {
			return nil, modelSettings{}, err
		}
		file, err := os.Open(paths[0])
		if err != nil {
			return nil, modelSettings{}, err
		}
		source = file
	}
	defer source.Close()
	file, config, err := readSettingsFile(source)
	if err != nil {
		return nil, modelSettings{}, err
	}
	result, err := app.showMessage(mygo.MessageOptions{
		Parent: win, Type: mygo.MessageQuestion, Title: locales.ImportSettings(),
		Message: locales.ConfirmSettingsImport(),
		Buttons: []string{locales.Cancel(), locales.ImportSettings()},
	})
	if err != nil || result.Button != 1 {
		return nil, modelSettings{}, err
	}
	return &file, config, nil
}

func (app *chatApp) applySettingsImport(file settingsFile, config modelSettings) error {
	if app.deferBrowserAction(func() {
		if err := app.applySettingsImport(file, config); err != nil {
			app.settingsImportResult(err)
		}
	}) {
		return nil
	}
	backend, err := prepareSettingsImport(app.store, file, config)
	if err != nil {
		return err
	}
	app.installSettingsImport(file, config, backend)
	return nil
}

func prepareSettingsImport(store *localStore, file settingsFile, config modelSettings) (*Chat, error) {
	backend := newChat(config)
	if backend.configErr != nil {
		return nil, backend.configErr
	}
	if err := store.importSettings(file, config); err != nil {
		if backend.network != nil {
			backend.network.CloseIdleConnections()
		}
		return nil, err
	}
	return backend, nil
}

func (app *chatApp) installSettingsImport(file settingsFile, config modelSettings, backend *Chat) {
	app.installConfiguration(config, backend, file.Models)
	app.providers = *file.Models
	app.providerError = ""
	app.language, app.busySend, app.systemPrompt = file.Language, file.BusySend, file.SystemPrompt
	previousShortcut := app.desktop.Shortcut
	app.desktop = file.Desktop
	app.clearConnectionTest()
	app.form = settingsForm{
		proxy: config.Proxy, providerDrafts: make(map[string]modelForm),
		modelForm: app.providerForm(app.providers.Selected.ProviderID),
	}
	app.extensionsForm = extensionsForm{}
	app.setMarkdownImageLoader(config.Proxy)
	if app.win == nil {
		return
	}
	app.applyLanguage()
	if app.desktop.Shortcut != previousShortcut {
		if app.registeredShortcut != "" {
			mygo.GlobalShortcut.Unregister(app.registeredShortcut)
		}
		app.registeredShortcut, app.actualShortcut, app.shortcutError = "", "", false
		app.startDesktopShortcut()
	}
}
