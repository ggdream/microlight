package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/pages/settings"
)

func (app *chatApp) settingsTransfer(c *ui.Context) {
	layout := windowLayout(c)
	settings.Card(c, locales.SettingsTransfer(), func() {
		ui.Text(c, locales.SettingsTransferHint()).FontSize(viewport.Font(c, 13)).TextColor(muted)
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(layout.X(8)).Children(func() {
			disabled := app.transferBusy || app.store == nil
			action := func(key, label, glyph string, click func()) {
				button := settings.ActionButton(c.Key(key), icons, label, glyph, disabled)
				if layout.Compact() {
					button.Grow(1).Basis(0).MinWidth(0).Justify(ui.Center).BorderWidth(0).
						Background(side)
					if button.Hovered() && !disabled {
						button.Background(tint)
					}
				}
				button.OnClick(click)
			}
			action("import", locales.ImportSettings(), "folder_open", app.chooseSettingsImport)
			action("export", locales.ExportSettings(), "download", app.chooseSettingsExport)
		})
		if status := app.transferStatus; status.message != "" {
			color := muted
			if status.failed {
				color = errorFG
			}
			ui.Text(c, status.message).FontSize(viewport.Font(c, 12)).TextColor(color)
		}
	})
}

func (app *chatApp) chooseSettingsExport() {
	if app.transferBusy || app.win == nil || app.store == nil {
		return
	}
	data, err := app.exportSettings()
	if err != nil {
		app.transferStatus = settingsStatus{message: locales.ExportSettingsFailed(), failed: true}
		return
	}
	app.transferBusy, app.transferStatus = true, settingsStatus{}
	win := app.win
	go func() {
		dir, _ := mygo.App.Path(mygo.PathDownloads)
		path, err := desktop.SaveDocument(mygo.SaveDialogOptions{
			Parent: win, Title: locales.ExportSettings(), DefaultPath: filepath.Join(dir, "microlight-settings.json"),
			Filters: []mygo.FileFilter{{Name: locales.SettingsFileFilter(), Extensions: []string{"json"}}},
		}, append(data, '\n'))
		win.Update(func() {
			app.transferBusy = false
			switch {
			case err != nil:
				app.transferStatus = settingsStatus{message: locales.ExportSettingsFailed(), failed: true}
			case path != "":
				app.transferStatus = settingsStatus{message: locales.SettingsExported()}
			}
		})
	}()
}

func (app *chatApp) chooseSettingsImport() {
	if app.transferBusy || app.win == nil || app.store == nil {
		return
	}
	app.transferBusy, app.transferStatus = true, settingsStatus{}
	win := app.win
	go func() {
		file, config, err := chooseSettingsFile(win)
		win.Update(func() {
			app.transferBusy = false
			if app.appContext().Err() != nil {
				return
			}
			if file != nil {
				err = app.applySettingsImport(*file, config)
			}
			switch {
			case err != nil:
				app.transferStatus = settingsStatus{message: locales.ImportSettingsFailed(), failed: true}
			case file != nil:
				app.transferStatus = settingsStatus{message: locales.SettingsImported()}
			}
		})
	}()
}

func chooseSettingsFile(win *mygo.Window) (*settingsFile, modelSettings, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: win, Title: locales.ImportSettings(),
		Filters: []mygo.FileFilter{{Name: locales.SettingsFileFilter(), Extensions: []string{"json"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, modelSettings{}, err
	}
	source, err := os.Open(paths[0])
	if err != nil {
		return nil, modelSettings{}, err
	}
	defer source.Close()
	file, config, err := readSettingsFile(source)
	if err != nil {
		return nil, modelSettings{}, err
	}
	result, err := mygo.Dialog.Message(mygo.MessageOptions{
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
	backend := newChat(config)
	if backend.configErr != nil {
		return backend.configErr
	}
	if err := app.store.importSettings(file, config); err != nil {
		if backend.network != nil {
			backend.network.CloseIdleConnections()
		}
		return err
	}
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
		return nil
	}
	app.applyLanguage()
	if app.desktop.Shortcut != previousShortcut {
		if app.registeredShortcut != "" {
			mygo.GlobalShortcut.Unregister(app.registeredShortcut)
		}
		app.registeredShortcut, app.actualShortcut, app.shortcutError = "", "", false
		app.startDesktopShortcut()
	}
	return nil
}
