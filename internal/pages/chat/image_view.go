package chat

import (
	"github.com/ggdream/microlight/pkg/viewport"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/internal/linkpreview"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/proxy"
)

const chatImageMaxHeight = 280

type markdownImage struct {
	done     chan struct{}
	bitmap   *ui.Bitmap
	original []byte
	filename string
}

func (app *chatApp) setMarkdownImageLoader(config proxy.Config) {
	if app.md == nil {
		return
	}
	app.markdownImageProxy = config
	app.markdownImages = make(map[string]*markdownImage)
	app.md.Block = func(c *ui.Context, doc *parser.Document, block int) bool {
		if doc.Block(block).Kind != parser.Paragraph {
			return false
		}
		inlines := doc.Inlines(block)
		if !isImageParagraph(inlines) {
			return false
		}
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			for i := 0; i < len(inlines); i = inlines[i].Next {
				node := inlines[i]
				if node.Kind != parser.Image {
					continue
				}
				alt := parser.PlainText(inlines[i+1 : node.Next])
				if node.Pending || node.Dest == "" {
					imagePlaceholder(c.Key(i), alt, false)
					continue
				}
				item := ui.Box(c.Key(i)).MaxWidthPercent(100)
				entry := app.scopedMarkdownImage(c, item, node.Dest)
				item.Children(func() {
					select {
					case <-entry.done:
						if entry.bitmap != nil {
							app.chatImage(c, entry, alt)
						} else {
							imagePlaceholder(c, alt, true)
						}
					default:
						imagePlaceholder(c, alt, false)
					}
				})
			}
		})
		return true
	}
}

func (app *chatApp) scopedMarkdownImage(c *ui.Context, item ui.Element, source string) *markdownImage {
	key := struct {
		Source string
		Proxy  proxy.Config
	}{source, app.markdownImageProxy}
	return *ui.Local(item, key, func() *markdownImage { return app.markdownImage(c, source) })
}

func (app *chatApp) markdownImage(c *ui.Context, source string) *markdownImage {
	if app.markdownImages == nil {
		app.markdownImages = make(map[string]*markdownImage)
	}
	if cached := app.markdownImages[source]; cached != nil {
		return cached
	}
	if len(app.markdownImages) >= 32 {
		clear(app.markdownImages)
	}
	image := &markdownImage{done: make(chan struct{})}
	app.markdownImages[source] = image
	services, ctx, config := c.Services(), app.appContext(), app.markdownImageProxy
	go func() {
		if decoded, err := linkpreview.FetchImage(ctx, source, config); err == nil {
			image.bitmap = ui.NewBitmap(decoded.Pixels)
			image.original = decoded.Original
			image.filename = imageFilename(source, decoded.Format)
		}
		close(image.done)
		services.Invalidate()
	}()
	return image
}

func isImageParagraph(inlines []parser.Inline) bool {
	found := false
	for i := 0; i < len(inlines); i = inlines[i].Next {
		switch node := inlines[i]; node.Kind {
		case parser.Image:
			found = true
		case parser.SoftBreak, parser.LineBreak:
		case parser.Text:
			if strings.TrimSpace(node.Text) != "" {
				return false
			}
		default:
			return false
		}
	}
	return found
}

func imagePlaceholder(c *ui.Context, alt string, failed bool) {
	label := locales.ImagePreviewLoading()
	if failed {
		label = locales.PreviewUnavailable()
	}
	ui.Text(c, label).Description(alt).FontSize(viewport.Font(c, 12)).TextColor(muted).
		Background(side).Padding(6, 10).Radius(6)
}

func (app *chatApp) chatImage(c *ui.Context, image *markdownImage, alt string) {
	w, h := image.bitmap.Size()
	height := min(float32(h), chatImageMaxHeight)
	ratio := float32(w) / float32(h)
	label := locales.ImagePreview()
	preview := func() { app.imagePreview = chatImagePreview{image: image, alt: alt} }
	ui.Themed(c, theme.PopupTheme(c)).Background(ui.Transparent).MaxWidthPercent(100).Children(func() {
		button := pointerButton(c, false).Label(label).Description(alt).Tooltip(label).
			Width(height * ratio).MaxWidthPercent(100).AspectRatio(ratio).MaxHeight(chatImageMaxHeight).
			Radius(7).Clip().FocusRingColor(accent).Children(func() {
			ui.Image(c, image.bitmap).Fill().Fit(ui.Contain).Label(alt)
		}).OnClick(preview)
		previewLabel, saveLabel := label, locales.DownloadImage()
		if windowLayout(c).Compact() {
			previewLabel, saveLabel = locales.PreviewImageAction(), locales.SaveImageAction()
		}
		var items []theme.PopupItem
		if desktop.Mobile {
			items = append(items, theme.PopupItem{Label: locales.ShareImage(), Icon: icons.Menu("share"), OnSelect: func() { app.shareImage(image.filename, image.original) }})
		}
		items = append(items,
			theme.PopupItem{Label: previewLabel, Icon: icons.Menu("open"), OnSelect: preview},
			theme.PopupItem{Label: saveLabel, Icon: icons.Menu("download"), OnSelect: func() { app.downloadImage(image) }},
		)
		theme.PopupContextMenu(button, items...)
	})
}

type chatImagePreview struct {
	image *markdownImage
	alt   string
}

func (app *chatApp) renderImagePreview(c *ui.Context) {
	preview := app.imagePreview
	if preview.image == nil {
		return
	}
	bitmap := preview.image.bitmap
	w, h := bitmap.Size()
	layout := windowLayout(c)
	actionSize := layout.Target(34)
	gap := layout.Y(8)
	imageWidth := layout.FitWidth(layout.Width, layout.X(layout.Density(28, 10)))
	imageHeight := layout.FitHeight(layout.Height, layout.Y(layout.Density(24, 10))) - actionSize - gap
	scale := min(max(1, imageWidth)/float32(w), max(1, imageHeight)/float32(h))
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		insetDialog(c, backdrop).Background(ui.RGBA(10, 18, 15, .88))
		panel.Label(locales.ImagePreview()).Description(preview.alt).Background(ui.Transparent)
		content := ui.Column(c).Gap(gap).AlignItems(ui.End)
		if layout.Compact() {
			panel.Width(layout.Width).Height(layout.Height).MaxWidthPercent(100).MaxHeightPercent(100).
				Padding(layout.Y(8), layout.X(10))
			content.Fill()
		}
		content.Children(func() {
			ui.Row(c.Key("actions")).FillWidth().Shrink(0).Justify(ui.End).AlignItems(ui.Center).Gap(layout.X(8)).Children(func() {
				imagePreviewAction(c.Key("download"), locales.DownloadImage(), "download", actionSize).
					OnClick(func() { app.downloadImage(preview.image) })
				imagePreviewAction(c.Key("close"), locales.ClosePreview(), "close", actionSize).AutoFocus().
					OnClick(func() { app.imagePreview = chatImagePreview{} })
			})
			picture := func() {
				ui.Image(c.Key("picture"), bitmap).Size(float32(w)*scale, float32(h)*scale).
					MaxWidthPercent(100).MaxHeightPercent(100).Fit(ui.Contain).Radius(7).Label(preview.alt)
			}
			if layout.Compact() {
				ui.Box(c.Key("image-area")).FillWidth().Grow(1).MinHeight(0).Center().Children(picture)
			} else {
				picture()
			}
		})
	})
	if !open {
		app.imagePreview = chatImagePreview{}
	}
}

func imagePreviewAction(c *ui.Context, label, glyph string, size float32) ui.Element {
	button := pointerButton(c, false).Label(label).Tooltip(label).Size(size, size).
		Radius(8).Center().Background(ink)
	if button.Hovered() {
		button.Background(muted)
	}
	return button.Children(func() { icon(c, glyph, white, 16) })
}
