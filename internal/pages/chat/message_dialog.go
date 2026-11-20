package chat

import (
	"errors"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/viewport"
)

type messageDialog struct {
	options mygo.MessageOptions
	result  chan mygo.MessageResult
}

// Called from workers, like the system message API it replaces. All queue and
// view mutations stay on the window thread; file pickers remain system owned.
func (app *chatApp) showMessage(options mygo.MessageOptions) (mygo.MessageResult, error) {
	if app.win == nil {
		return mygo.MessageResult{}, errors.New("message dialog requires a window")
	}
	request := &messageDialog{options: options, result: make(chan mygo.MessageResult, 1)}
	app.win.Update(func() {
		if app.appContext().Err() == nil {
			app.messageDialogs = append(app.messageDialogs, request)
		}
	})
	select {
	case result := <-request.result:
		return result, nil
	case <-app.appContext().Done():
		return mygo.MessageResult{}, app.appContext().Err()
	}
}

func (app *chatApp) messageDialog(c *ui.Context) {
	if len(app.messageDialogs) == 0 {
		return
	}
	request := app.messageDialogs[0]
	options := request.options
	buttons := options.Buttons
	if len(buttons) == 0 {
		buttons = []string{locales.CloseChatInfo()}
	}
	finish := func(index int) {
		app.messageDialogs = app.messageDialogs[1:]
		request.result <- mygo.MessageResult{Button: index}
	}
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		insetDialog(c, backdrop).Background(material3.Monochrome.Scrim.Alpha(.32))
		panel.Label(options.Title).Width(420).MaxWidthPercent(90).MaxHeight(dialogMaxHeight(c)).Padding(24).Gap(16)
		mobileDialog(c, backdrop, panel)
		dialogBody(c, func() {
			if options.Title != "" {
				material3.Text(c, options.Title, material3.TitleLarge)
			}
			ui.Text(c, options.Message).FontSize(viewport.Font(c, 14)).TextColor(ink)
			if options.Detail != "" {
				ui.Text(c, options.Detail).FontSize(viewport.Font(c, 13)).TextColor(muted)
			}
		})
		dialogActions(c).Children(func() {
			for index, label := range buttons {
				variant := material3.TextButton
				if len(buttons) > 1 && index == len(buttons)-1 {
					variant = material3.FilledButton
				}
				button := material3.Button(c.Key(index), label, variant, false).Height(windowLayout(c).Target(36))
				if index == options.DefaultButton {
					button.AutoFocus()
				}
				button.Children(func() {
					material3.Text(c, label, material3.LabelLarge).TextColor(material3.Foreground(variant, false))
				}).OnClick(func() { finish(index) })
			}
		})
	})
	if !open {
		finish(options.CancelButton)
	}
}
