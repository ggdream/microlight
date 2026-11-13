package chat

import (
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func (app *chatApp) composer(c *ui.Context, selected *conversation) {
	ui.Column(c).Shrink(0).AlignItems(ui.Center).Padding(12, 22, 18).Gap(7).Children(func() {
		if app.draftErr != "" {
			ui.Text(c, app.draftErr).FontSize(11).TextColor(errorFG)
		}
		if app.accessError != "" {
			ui.Text(c, app.accessError).FontSize(11).TextColor(errorFG)
		}
		if app.providerError != "" {
			ui.Text(c, app.providerError).FontSize(11).TextColor(errorFG)
		}
		if app.historyErr != "" {
			ui.Text(c, app.historyErr).FontSize(11).TextColor(errorFG)
		}
		if app.backend.configErr != nil {
			ui.Text(c, app.backend.configErr.Error()).FontSize(11).TextColor(errorFG)
		}
		runningHere := app.active != nil && app.active == selected
		queueDraft := app.shouldQueue(selected)
		app.messageQueue(c.Key("message-queue"), selected)
		ui.Column(c).FillWidth().MaxWidth(conversationWidth).Background(white).
			Border(1, border).Radius(17).Shadow(0, 4, 14, 0, ui.RGBA(29, 49, 38, .06)).
			Padding(11, 12, 9).Gap(5).Children(func() {
			if selected.waitingReply() != nil {
				if app.skillPicker.open {
					app.resetComposerSkills()
				}
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					app.workspaceControl(c, selected)
					app.accessControl(c, selected)
				})
				return
			}
			if len(app.draftAttachments) > 0 {
				ui.ScrollHorizontal(c).FillWidth().Height(77).Children(func() {
					ui.Row(c).Gap(8).Children(func() {
						for i, attachment := range app.draftAttachments {
							attachmentTile(c.Key(attachment), attachment, 62, func() {
								app.draftAttachments = slices.Delete(app.draftAttachments, i, i+1)
								app.attachmentError = ""
							})
						}
					})
				})
			}
			if app.attachmentError != "" {
				ui.Text(c, app.attachmentError).FontSize(11).TextColor(errorFG)
			}
			placeholder := locales.MessagePlaceholder()
			if queueDraft {
				placeholder = locales.QueuePlaceholder()
			} else if runningHere {
				placeholder = locales.SteerPlaceholder()
			}
			app.draft.DiscardFilePasteText()
			draftHeight := float32(min(135, 20+19*app.draft.Document.Text().ParagraphCount()))
			editor := app.draft.View(c.Key("draft"), locales.Message(), placeholder, draftHeight, app.md).
				Padding(3, 3, 0).FocusRing(false)
			services := c.Services()
			editor.ObserveInput(func(ev ui.InputEvent) { app.pasteAttachments(services, ev) })
			workspace := app.draftWorkspace
			if selected != nil {
				workspace = selected.workspace
			}
			pickingSkill := app.composerSkills(c.Key("skill-picker"), editor, workspace)
			if !pickingSkill && runningHere && editor.Shortcut(0, ui.KeyEscape) {
				app.stopReply(selected, app.running)
			}
			if editor.Shortcut(ui.Shift, ui.KeyEnter) {
				app.draft.Document.InsertText("\n")
			}
			if runningHere && queueDraft && editor.Shortcut(ui.Ctrl, ui.KeyEnter) {
				app.send(app.draft.Markdown(), true)
			} else if !pickingSkill && editor.Shortcut(0, ui.KeyEnter) {
				app.send(app.draft.Markdown(), false)
			}
			ui.Row(c).AlignItems(ui.Center).PaddingX(2).Gap(8).Children(func() {
				app.composerMenu(c.Key("composer-menu"), workspace)
				app.workspaceControl(c, selected)
				app.accessControl(c, selected)
				sendLabel, sendIcon := locales.Send(), "up"
				if queueDraft {
					sendLabel, sendIcon = locales.QueueMessage(), "queue_add"
				} else if runningHere {
					sendLabel, sendIcon = locales.Steer(), "queue_steer"
				}
				if app.active != nil && !runningHere {
					label := locales.OtherChatGenerating()
					ui.Text(c, label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(label).FontSize(10).TextColor(muted)
				} else {
					ui.Box(c).Grow(1)
				}
				app.chatModelSelect(c.Key("model"))
				canSend := !app.loadingAttachments && app.backend.configErr == nil &&
					(strings.TrimSpace(app.draft.Document.Text().String()) != "" || len(app.draftAttachments) != 0)
				if runningHere {
					reply := app.running
					stopButton(c).OnClick(func() { app.stopReply(selected, reply) })
					if canSend {
						actionButton(c, sendLabel, sendIcon, false).OnClick(func() { app.send(app.draft.Markdown(), false) })
					}
				} else {
					actionButton(c, sendLabel, sendIcon, !canSend || app.active != nil).OnClick(func() { app.send(app.draft.Markdown(), false) })
				}
			})
		})
	})
}
