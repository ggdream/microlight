package chat

import (
	"strings"

	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/assets/locales"

	"github.com/egoist/mygo/ui"
)

func attachmentTile(c *ui.Context, attachment *attachment.File, size float32, remove func()) {
	isImage := attachment.IsImage()
	isText := !isImage && attachment.IsText()
	tooltip := attachment.Name
	if !isImage && !isText {
		tooltip += "\n" + locales.AttachmentNameOnly()
	}
	tile := ui.Column(c).Size(size, size).Shrink(0).Radius(7).Clip().Background(side).
		AlignItems(ui.Center).Justify(ui.Center).Tooltip(tooltip)
	if !isImage {
		tile.Background(white).Border(1, border)
	}
	fit := ui.Contain
	showRemove := false
	if remove != nil {
		fit = ui.Cover
		tile.Focusable().Label(locales.AttachmentLabel(attachment.Name)).FocusRingColor(accent)
		showRemove = tile.Hovered() || tile.FocusWithin()
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
					ui.Text(c, excerpt).FillWidth().Padding(5).Font("monospace").FontSize(7).TextColor(muted).
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
			ui.Row(c).FillWidth().Height(18).Shrink(0).PaddingX(5).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(border).Children(func() {
				ui.Text(c, attachment.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Clip().
					FontSize(9).TextColor(ink)
			})
		}
		if showRemove {
			button := pointerButton(c, false).Label(locales.RemoveAttachment(attachment.Name)).Absolute().Top(3).Right(3).
				Width(18).Height(18).Radius(9).Background(ui.RGBA(20, 25, 23, .8)).
				Border(1, ui.RGBA(255, 255, 255, .65)).AlignItems(ui.Center).Justify(ui.Center)
			button.Children(func() { icon(c, "close", white, 11) })
			if button.Hovered() {
				button.Background(ink)
			}
			button.OnClick(remove)
		}
	})
}
