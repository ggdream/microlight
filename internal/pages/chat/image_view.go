package chat

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/ggdream/microlight/assets/locales"
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
	ctx := app.appContext()
	cache := make(map[string]*markdownImage)
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
				entry := ui.Local(item, struct {
					Source string
					Proxy  proxy.Config
				}{node.Dest, config}, func() *markdownImage {
					if cached := cache[node.Dest]; cached != nil {
						return cached
					}
					if len(cache) >= 32 {
						clear(cache)
					}
					image := &markdownImage{done: make(chan struct{})}
					cache[node.Dest] = image
					services := c.Services()
					go func() {
						if decoded, err := linkpreview.FetchImage(ctx, node.Dest, config); err == nil {
							image.bitmap = ui.NewBitmap(decoded.Pixels)
							image.original = decoded.Original
							image.filename = imageFilename(node.Dest, decoded.Format)
						}
						close(image.done)
						services.Invalidate()
					}()
					return image
				})
				item.Children(func() {
					select {
					case <-(*entry).done:
						if (*entry).bitmap != nil {
							app.chatImage(c, *entry, alt)
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
	ui.Text(c, label).Description(alt).FontSize(12).TextColor(muted).
		Background(side).Padding(6, 10).Radius(6)
}

func (app *chatApp) chatImage(c *ui.Context, image *markdownImage, alt string) {
	w, h := image.bitmap.Size()
	height := min(float32(h), chatImageMaxHeight)
	ratio := float32(w) / float32(h)
	label := locales.ImagePreview()
	preview := func() { app.imagePreview = chatImagePreview{image: image, alt: alt} }
	ui.Themed(c, theme.MenuTheme).Background(ui.Transparent).MaxWidthPercent(100).Children(func() {
		button := pointerButton(c, false).Label(label).Description(alt).Tooltip(label).
			Width(height * ratio).MaxWidthPercent(100).AspectRatio(ratio).MaxHeight(chatImageMaxHeight).
			Radius(7).Clip().FocusRingColor(accent).Children(func() {
			ui.Image(c, image.bitmap).Fill().Fit(ui.Contain).Label(alt)
		}).OnClick(preview)
		theme.PopupContextMenu(button,
			theme.PopupItem{Label: label, Icon: icons.Menu("open"), OnSelect: preview},
			theme.PopupItem{Label: locales.DownloadImage(), Icon: icons.Menu("download"), OnSelect: func() { app.downloadImage(image) }},
		)
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
	width, height := c.Size()
	scale := min(max(1, width-56)/float32(w), max(1, height-96)/float32(h))
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(10, 18, 15, .88))
		panel.Label(locales.ImagePreview()).Description(preview.alt).Background(ui.Transparent)
		ui.Column(c).Gap(8).AlignItems(ui.End).Children(func() {
			ui.Row(c.Key("actions")).AlignItems(ui.Center).Gap(8).Children(func() {
				imagePreviewAction(c.Key("download"), locales.DownloadImage(), "download").
					OnClick(func() { app.downloadImage(preview.image) })
				imagePreviewAction(c.Key("close"), locales.ClosePreview(), "close").AutoFocus().
					OnClick(func() { open = false })
			})
			ui.Image(c.Key("picture"), bitmap).Size(float32(w)*scale, float32(h)*scale).
				Fit(ui.Contain).Radius(7).Label(preview.alt)
		})
	})
	if !open {
		app.imagePreview = chatImagePreview{}
	}
}

func imagePreviewAction(c *ui.Context, label, glyph string) ui.Element {
	button := pointerButton(c, false).Label(label).Tooltip(label).Size(34, 34).
		Radius(17).Center().Background(ink)
	if button.Hovered() {
		button.Background(muted)
	}
	return button.Children(func() { icon(c, glyph, white, 16) })
}
