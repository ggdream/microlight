package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
)

func (app *chatApp) composer(c *ui.Context, selected *conversation) {
	layout := app.composerLayout(c)
	gap := layout.Density(7, 3)
	controlSize := composerControlHeight(layout, 32)
	short := layout.Height > 0 && layout.Height < 420
	verticalInset := layout.Y(layout.Density(17, 16))
	horizontalInset := layout.X(layout.Density(12, 7))
	bottomInset := layout.Y(layout.Density(18, 6))
	if layout.Compact() && selected != nil && !app.desktop.HideStatistics && selected.metrics().Requests > 0 {
		bottomInset = 0
	}
	ui.Column(c).Shrink(0).AlignItems(ui.Center).Padding(layout.Y(layout.Density(12, 5)), layout.X(layout.Density(22, 14)), bottomInset).Gap(gap).Children(func() {
		if app.draftErr != "" {
			ui.Text(c, app.draftErr).FontSize(viewport.Font(c, 11)).TextColor(errorFG)
		}
		if app.accessError != "" {
			ui.Text(c, app.accessError).FontSize(viewport.Font(c, 11)).TextColor(errorFG)
		}
		if app.providerError != "" {
			ui.Text(c, app.providerError).FontSize(viewport.Font(c, 11)).TextColor(errorFG)
		}
		if selected != nil && selected.runError != "" {
			ui.Text(c, selected.runError).FontSize(viewport.Font(c, 11)).TextColor(errorFG)
		}
		if app.historyErr != "" {
			ui.Text(c, app.historyErr).FontSize(viewport.Font(c, 11)).TextColor(errorFG)
		}
		if app.backend.configErr != nil {
			ui.Text(c, app.backend.configErr.Error()).FontSize(viewport.Font(c, 11)).TextColor(errorFG)
		}
		var run *replyRun
		if selected != nil {
			run = selected.run
		}
		runningHere := run != nil
		queueDraft := app.shouldQueue(selected)
		sendControl := func() {
			sendLabel, sendIcon := locales.Send(), "up"
			if queueDraft {
				sendLabel, sendIcon = locales.QueueMessage(), "queue_add"
			} else if runningHere {
				sendLabel, sendIcon = locales.Steer(), "queue_steer"
			}
			canSend := !app.loadingAttachments && app.backend.configErr == nil && (strings.TrimSpace(app.draft.Document.Text().String()) != "" || len(app.draftAttachments) != 0)
			if runningHere {
				stopButton(c).Size(controlSize, controlSize).OnClick(func() { app.stopReply(selected, run.reply) })
				if !canSend {
					return
				}
			}
			actionButton(c, sendLabel, sendIcon, !canSend).Size(controlSize, controlSize).OnClick(func() { app.send(app.draft.Markdown(), false) })
		}
		app.messageQueue(c.Key("message-queue"), selected)
		material3.Elevate(material3.CardStyle(ui.Column(c).FillWidth().MaxWidth(conversationWidth), material3.ElevatedCard).
			Radius(material3.ShapeMedium), 2).
			Padding(verticalInset, horizontalInset, layout.Y(layout.Density(9, 8))).Gap(layout.Y(layout.Density(5, 6))).Children(func() {
			if app.edit != nil && app.edit.inComposer {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Box(c).Grow(1)
					quietButton(c, locales.Cancel(), "close").OnClick(app.cancelComposerEdit)
				})
			}
			if selected.waitingReply() != nil {
				app.focusComposer = false
				if app.skillPicker.open {
					app.resetComposerSkills()
				}
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					app.accessControl(c, selected)
				})
				return
			}
			if len(app.draftAttachments) > 0 {
				tileSize := float32(62)
				if short {
					tileSize = 44
				}
				scroll, offset := elastic.Horizontal(c)
				scroll.FillWidth().Height(tileSize + 10).Children(func() {
					offset.Apply(ui.Row(c)).Gap(8).Children(func() {
						for i, attachment := range app.draftAttachments {
							app.attachmentTile(c.Key(attachment), attachment, tileSize, func() {
								app.draftAttachments = slices.Delete(app.draftAttachments, i, i+1)
								app.attachmentError = ""
							})
						}
					})
				})
			}
			if app.attachmentError != "" {
				ui.Text(c, app.attachmentError).FontSize(viewport.Font(c, 11)).TextColor(errorFG)
			}
			placeholder := locales.MessagePlaceholder()
			if queueDraft {
				placeholder = locales.QueuePlaceholder()
			} else if runningHere {
				placeholder = locales.SteerPlaceholder()
			}
			app.draft.DiscardFilePasteText()
			lines := max(1, app.draft.Document.Text().ParagraphCount())
			baseHeight := layout.Text(layout.Density(39, 34))
			maxDraftHeight := layout.Y(135)
			if short {
				maxDraftHeight = max(baseHeight, layout.Height*.28)
			}
			draftHeight := min(maxDraftHeight, baseHeight+layout.Text(19)*float32(lines-1))
			multiline := lines > 1
			if layout.Compact() {
				available := layout.Width - layout.X(28) - 2*horizontalInset - layout.X(12) - controlSize - 2
				if runningHere && strings.TrimSpace(app.draft.Document.Text().String()) != "" {
					available -= controlSize + layout.X(6)
				}
				multiline = multiline || app.draft.SingleLineWidth(c) > available
			}
			var editor ui.Element
			ui.Row(c.Key("draft-row")).FillWidth().AlignItems(ui.Center).Gap(layout.X(6)).Children(func() {
				editor = app.draft.View(c.Key("draft"), locales.Message(), placeholder, draftHeight, app.md).
					Padding(layout.Y(layout.Density(3, 5)), layout.X(3), 0).Grow(1).MinWidth(0).Shrink(0).Clip().FocusRing(false)
				if layout.Compact() && !multiline {
					sendControl()
				}
			})
			// Wrapped lines do not increase ParagraphCount. Size from the editor's
			// shaped content so its internal scroll cannot pull text into the inset.
			if contentHeight, ok := app.draft.Document.ContentHeight(); ok {
				draftHeight = min(maxDraftHeight, max(baseHeight, contentHeight+layout.Y(5)))
				editor.Height(draftHeight)
			}
			if height := editor.LayoutBounds().H; height < draftHeight-.5 || height > draftHeight+.5 {
				c.AnimationFrame()
			}
			if !desktop.Mobile {
				editor.AutoFocus()
			}
			if app.focusComposer && app.win != nil && app.win.IsFocused() && mygo.App.IsActive() {
				editor.Focus()
				app.focusComposer = false
			}
			services := c.Services()
			editor.ObserveInput(func(ev ui.InputEvent) { app.pasteAttachments(services, ev) })
			workspace := app.draftWorkspace
			if selected != nil {
				workspace = selected.workspace
			}
			pickingSkill := app.composerSkills(c.Key("skill-picker"), editor, workspace)
			if !pickingSkill && runningHere && editor.Shortcut(0, ui.KeyEscape) {
				app.stopReply(selected, run.reply)
			}
			if editor.Shortcut(ui.Shift, ui.KeyEnter) || (desktop.Mobile && editor.Shortcut(0, ui.KeyEnter)) {
				app.draft.Document.InsertText("\n")
			}
			if runningHere && queueDraft && editor.Shortcut(ui.Ctrl, ui.KeyEnter) {
				app.send(app.draft.Markdown(), true)
			} else if !desktop.Mobile && !pickingSkill && editor.Shortcut(0, ui.KeyEnter) {
				app.send(app.draft.Markdown(), false)
			}
			ui.Row(c).AlignItems(ui.Center).PaddingX(layout.Density(2, 0)).Gap(layout.X(layout.Density(8, 4))).Children(func() {
				app.composerMenu(c.Key("composer-menu"), workspace)
				app.workspaceControl(c, selected)
				app.accessControl(c, selected)
				ui.Box(c).Grow(1)
				app.chatModelSelect(c.Key("model"))
				if !layout.Compact() || multiline {
					sendControl()
				}
			})
		})
		app.conversationStatistics(c.Key("conversation-statistics"), selected)
	})
}
