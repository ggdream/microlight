package settings

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/theme"
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
	ui.Column(c).FillWidth().Background(theme.White).Radius(13).Border(1, theme.Border).
		Padding(15, 16).Gap(11).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, locales.OcrTool()).FontSize(15).FontWeight(660)
				ui.Text(c, locales.OcrToolHint()).FontSize(12).TextColor(theme.Muted)
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
			ui.Text(c, form.err).FontSize(12).TextColor(theme.ErrorFG)
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
