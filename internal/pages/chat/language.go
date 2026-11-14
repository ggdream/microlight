package chat

import (
	"log/slog"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/storage"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (app *chatApp) loadLanguage() {
	app.language = settings.SystemLanguage
	if app.store != nil {
		saved, err := app.store.getConfig("language")
		if err != nil {
			slog.Error("read language preference", "error", err)
		} else {
			switch settings.LanguagePreference(saved) {
			case settings.English, settings.Chinese:
				app.language = settings.LanguagePreference(saved)
			case settings.SystemLanguage:
			default:
				if err := storage.PutConfig(app.store.DB, "language", string(settings.SystemLanguage)); err != nil {
					slog.Error("save default language preference", "error", err)
				}
			}
		}
	}
	app.applyLanguage()
}

func (app *chatApp) applyLanguage() {
	if app.language == settings.SystemLanguage {
		mygo.App.SetLocale("")
		return
	}
	mygo.App.SetLocale(string(app.language))
}

func (app *chatApp) chooseLanguage(language settings.LanguagePreference) {
	if app.store == nil || storage.PutConfig(app.store.DB, "language", string(language)) != nil {
		app.form.generalStatus.message, app.form.generalStatus.failed = locales.LanguageSaveFailed(), true
		return
	}
	app.language = language
	app.form.generalStatus.message, app.form.generalStatus.failed = "", false
	app.applyLanguage()
}

func (app *chatApp) languageSettings(c *ui.Context) {
	settings.Language(c, icons, app.language, app.chooseLanguage)
}
