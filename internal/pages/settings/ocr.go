package settings

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/ocr"
)

type OCRForm struct {
	draft *localtools.OCRConfig
	err   string
}

func (form OCRForm) HasChanges(saved localtools.OCRConfig) bool {
	saved.Provider = saved.Selected()
	return form.draft != nil && *form.draft != saved
}

func (form *OCRForm) View(c *ui.Context, icons asseticon.Set, saved localtools.OCRConfig, save func(localtools.OCRConfig) error) {
	if form.draft == nil {
		draft := saved
		draft.Provider = saved.Selected()
		form.draft = &draft
	}
	draft := form.draft
	resetError := func() { form.err = "" }
	material3.CardStyle(ui.Column(c).FillWidth(), material3.ElevatedCard).
		Padding(settingY(c, 15), settingX(c, 16)).Gap(settingGap(c, 11)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(settingsLayout(c).Density(12, 8)).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(settingGap(c, 4)).Children(func() {
				ui.Text(c, locales.OcrTool()).FontSize(viewport.Font(c, 15)).FontWeight(660)
				ui.Text(c, locales.OcrToolHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
			})
			theme.Switch(c.Key("enabled"), &draft.Enabled, locales.EnableOcrTool(), resetError)
		})
		settingsSelect(c.Key("provider"), icons, locales.OcrProvider(), &draft.Provider,
			[]localtools.OCRProvider{localtools.PaddleOCR}, ocrProviderLabel, resetError)
		switch draft.Provider {
		case localtools.PaddleOCR:
			paddleOCRFields(c.Key("paddleocr"), icons, draft, resetError)
		}
		if form.err != "" {
			ui.Text(c, form.err).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
		}
		if form.HasChanges(saved) {
			ui.Row(c).Justify(ui.End).Children(func() {
				ActionButton(c.Key("save"), icons, locales.SaveSettings(), "check", false).OnClick(func() {
					config := *draft
					config.Token = strings.TrimSpace(config.Token)
					if err := save(config); err != nil {
						form.err = err.Error()
						return
					}
					*form = OCRForm{}
				})
			})
		}
	})
}

func ocrProviderLabel(provider localtools.OCRProvider) string {
	if provider == localtools.PaddleOCR {
		return "PaddleOCR"
	}
	return string(provider)
}

func paddleOCRFields(c *ui.Context, icons asseticon.Set, draft *localtools.OCRConfig, changed func()) {
	textModel, documentModel := draft.SelectedTextModel(), draft.SelectedDocumentModel()
	modelLabel := func(model ocr.Model) string { return string(model) }
	Field(c.Key("token"), locales.OcrToken(), "", &draft.Token).Password().OnChange(changed)
	settingsSelect(c.Key("text-model"), icons, locales.OcrTextModel(), &textModel, ocr.TextModels(), modelLabel, func() {
		draft.TextModel = textModel
		changed()
	})
	settingsSelect(c.Key("document-model"), icons, locales.OcrDocumentModel(), &documentModel, ocr.DocumentModels(), modelLabel, func() {
		draft.DocumentModel = documentModel
		changed()
	})
}
