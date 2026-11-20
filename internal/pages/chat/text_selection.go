package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/material3"
)

// Native read-only inputs provide touch selection handles and the system copy
// menu. Keep Markdown in the transcript and expose its source on long press.
func (app *chatApp) selectMessageText(c *ui.Context, element ui.Element, text string) {
	if touchControls(c) && element.LongPressed() {
		value := text
		app.selectionText = &value
	}
}

func (app *chatApp) textSelectionDialog(c *ui.Context) {
	if app.selectionText == nil {
		return
	}
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		insetDialog(c, backdrop).Background(material3.Monochrome.Scrim.Alpha(.32))
		panel.Width(420).MaxHeight(dialogMaxHeight(c)).Padding(16).Gap(12).Label(locales.CopyMessage())
		mobileDialog(c, backdrop, panel)
		ui.TextArea(c, app.selectionText).ReadOnly(true).FillWidth().Height(windowLayout(c).FitHeight(360, 100)).FontSize(layoutMessageFont(c))
		dialogActions(c).Children(func() {
			clarificationButton(c, locales.Copy(), false, func() { c.WriteClipboard(*app.selectionText) })
			clarificationButton(c, locales.CloseChatInfo(), false, func() { app.selectionText = nil })
		})
	})
	if !open {
		app.selectionText = nil
	}
}
