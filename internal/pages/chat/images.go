package chat

import (
	"path/filepath"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/desktop"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func (app *chatApp) pickAttachments() {
	if desktop.Web {
		app.pickBrowserAttachments()
		return
	}
	app.addDraftAttachments(func() ([]string, []byte, error) {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: app.win, Title: locales.AddAttachment(), ButtonLabel: locales.Add(), Multiple: true,
		})
		return paths, nil, err
	})
}

func (app *chatApp) pickBrowserAttachments() {
	if app.loadingAttachments || app.win == nil {
		return
	}
	slots := attachment.MaxPerMessage - len(app.draftAttachments)
	if slots <= 0 {
		app.attachmentError = locales.AttachmentCountLimit(attachment.MaxPerMessage)
		return
	}
	remaining := attachment.MaxDraftBytes
	for _, file := range app.draftAttachments {
		remaining -= len(file.Data)
	}
	app.loadingAttachments = true
	version, win := app.draftVersion, app.win
	go func() {
		files, err := desktop.OpenBrowserFiles(true, "", attachment.MaxBytes)
		var added []*attachment.File
		var issues []string
		if err != nil {
			issues = append(issues, locales.FilePickerFailed())
		}
		for _, source := range files {
			if len(added) >= slots {
				issues = append(issues, locales.AttachmentCountLimit(attachment.MaxPerMessage))
				break
			}
			file, loadErr := attachment.New(source.Name, source.Data)
			if loadErr != nil {
				issues = append(issues, source.Name+": "+loadErr.Error())
				continue
			}
			if len(file.Data) > remaining {
				issues = append(issues, locales.AttachmentsTooLarge())
				break
			}
			added = append(added, file)
			remaining -= len(file.Data)
		}
		win.Update(func() {
			app.loadingAttachments = false
			if app.draftVersion == version {
				app.draftAttachments = append(app.draftAttachments, added...)
				app.attachmentError = strings.Join(issues, "；")
			}
		})
	}()
}

func (app *chatApp) pasteAttachments(services ui.Services, ev ui.InputEvent) {
	app.draft.DiscardFilePasteText()
	pasteCommand := ev.Kind == ui.InputCommand && (ev.Text == "paste" || ev.Text == "pasteAndMatchStyle")
	pasteKey := ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyV &&
		(ev.Mods == ui.Cmd || ev.Mods == ui.Cmd|ui.Shift || ev.Mods == ui.Cmd|ui.Shift|ui.Alt)
	if (!pasteCommand && !pasteKey) || app.win == nil {
		return
	}
	paths, _ := services.ReadClipboardData(transfer.FileList, transfer.URIList).Files()
	var png []byte
	if len(paths) == 0 {
		png, _ = services.ReadClipboardData(transfer.PNG).Read(transfer.PNG)
	}
	if len(paths) != 0 || len(png) != 0 {
		app.draft.MarkFilePaste()
		if !ev.Repeat {
			app.addDraftAttachments(func() ([]string, []byte, error) { return paths, png, nil })
		}
	}
}

func (app *chatApp) addDraftAttachments(source func() ([]string, []byte, error)) {
	if app.loadingAttachments {
		return
	}
	slots := attachment.MaxPerMessage - len(app.draftAttachments)
	if slots <= 0 {
		app.attachmentError = locales.AttachmentCountLimit(attachment.MaxPerMessage)
		return
	}
	app.loadingAttachments = true
	version := app.draftVersion
	remaining := attachment.MaxDraftBytes
	for _, attachment := range app.draftAttachments {
		remaining -= len(attachment.Data)
	}
	go func() {
		paths, png, err := source()
		var attachments []*attachment.File
		var issues []string
		if err != nil {
			issues = append(issues, locales.FilePickerFailed())
		}
		add := func(name string, attachment *attachment.File, err error) {
			switch {
			case err != nil:
				issues = append(issues, name+": "+err.Error())
			case len(attachment.Data) > remaining:
				issues = append(issues, name+": "+locales.AttachmentsTooLarge())
			default:
				attachments = append(attachments, attachment)
				remaining -= len(attachment.Data)
			}
		}
		if len(png) != 0 {
			name := locales.PastedImageName()
			attachment, err := attachment.New(name, png)
			add(name, attachment, err)
		}
		for _, path := range paths {
			if len(attachments) == slots {
				issues = append(issues, locales.AttachmentCountLimit(attachment.MaxPerMessage))
				break
			}
			attachment, err := attachment.Load(path)
			add(filepath.Base(path), attachment, err)
		}
		app.win.Update(func() {
			app.loadingAttachments = false
			if app.draftVersion == version && (len(paths) > 0 || len(png) != 0 || err != nil) {
				app.draftAttachments = append(app.draftAttachments, attachments...)
				app.attachmentError = strings.Join(issues, "；")
			}
		})
	}()
}
