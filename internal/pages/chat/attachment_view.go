package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"path/filepath"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

func (app *chatApp) attachmentTile(c *ui.Context, attachment *attachment.File, size float32, remove func()) {
	touch := touchControls(c)
	width := size
	ui.Themed(c, theme.PopupTheme(c)).Background(ui.Transparent).Size(width, size).Shrink(0).
		Children(func() { app.attachmentCard(c.Key("file"), attachment, size, touch, remove) })
}

func (app *chatApp) attachmentCard(c *ui.Context, attachment *attachment.File, size float32, touch bool, remove func()) {
	tile := ui.Row(c).Fill().AlignItems(ui.Center)
	showRemove := remove != nil && (touch || tile.Hovered() || tile.FocusWithin())
	tile.Children(func() {
		app.attachmentPreviewTile(c.Key("preview"), attachment, size, touch, remove)
		if !showRemove {
			return
		}
		button := pointerButton(c.Key("remove"), false).Label(locales.RemoveAttachment(attachment.Name)).Center().Shrink(0)
		hit := float32(18)
		if touch {
			hit = min(size, windowLayout(c).Target(44))
		}
		button.Absolute().Top(0).Right(0).Size(hit, hit)
		button.Children(func() {
			face := material3.Elevate(ui.Box(c).Absolute().Top(3).Right(3).Size(20, 20).Radius(10).
				Background(ink.Alpha(.85)).Center(), 1)
			if button.Hovered() || button.Pressed() {
				face.Background(ink)
			}
			face.Children(func() { icon(c, "close", white, 12) })
		})
		button.OnClick(remove)
	})
}

func (app *chatApp) attachmentPreviewTile(c *ui.Context, attachment *attachment.File, size float32, touch bool, remove func()) {
	isImage := attachment.IsImage()
	isText := !isImage && attachment.IsText()
	tile := ui.Column(c).Size(size, size).Shrink(0).Radius(7).Clip().Background(side).
		AlignItems(ui.Center).Justify(ui.Center).
		Focusable().Label(locales.AttachmentLabel(attachment.Name)).FocusRingColor(accent)
	if filepath.IsAbs(attachment.Path) {
		tile.Tooltip(attachment.Path)
	}
	preview := func() { app.showPreviewError(app.quickLookAttachment(attachment)) }
	if touch {
		tile.Role(ui.RoleButton).OnClick(preview)
	}
	if tile.Focused() && tile.Shortcut(0, ui.KeySpace) {
		preview()
	}
	if !isImage {
		material3.Elevate(tile.Background(white), 1).BorderWidth(0)
	}
	fit := ui.Contain
	if remove != nil {
		fit = ui.Cover
	}
	tile.Children(func() {
		if isImage {
			if thumb := attachment.Thumbnail(); thumb != nil {
				ui.Image(c, thumb).Label(attachment.Name).Fill().Fit(fit)
			} else {
				icon(c, "image", muted, 16)
			}
		} else {
			ui.Box(c).FillWidth().Grow(1).MinHeight(0).Clip().Children(func() {
				if isText {
					excerpt := strings.TrimSpace(strings.ToValidUTF8(string(attachment.Data[:min(len(attachment.Data), 120)]), ""))
					ui.Text(c, excerpt).FillWidth().Padding(5).Font("monospace").FontSize(viewport.Font(c, 7)).TextColor(muted).
						MaxLines(3).Ellipsis("…")
					return
				}
				color := muted
				if attachment.MIME == "application/pdf" {
					color = errorFG
				}
				ui.Box(c).Fill().Background(side).Center().Children(func() {
					icon(c, "document_text", color, size*.49)
				})
			})
			material3.EdgeDivider(ui.Row(c).FillWidth().Height(18).Shrink(0).PaddingX(5).AlignItems(ui.Center), material3.TopEdge).
				Children(func() {
					ui.Text(c, attachment.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Clip().
						FontSize(9).TextColor(ink)
				})
		}
	})
	items := []theme.PopupItem{{Label: desktop.PreviewLabel(), Icon: icons.Menu("open"), OnSelect: preview}}
	if desktop.Mobile && isImage {
		items[0] = theme.PopupItem{Label: locales.ShareImage(), Icon: icons.Menu("share"), OnSelect: func() { app.shareImage(attachment.Name, attachment.Data) }}
	}
	if remove != nil {
		items = append(items, theme.PopupItem{Label: locales.DetachFile(), Icon: icons.Menu("close"), OnSelect: remove})
	}
	theme.PopupContextMenu(tile, items...)
}
