package markup

import (
	"errors"
	"math"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/f1sherb0y/mermaid-go"
	_ "github.com/f1sherb0y/mermaid-go/diagrams"
	"github.com/f1sherb0y/mermaid-go/scene"
	"github.com/f1sherb0y/mermaid-go/text"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

type diagramImage struct {
	shapes  *ui.SVG
	box     scene.Rect
	labels  []diagramLabel
	shadows []diagramShadow
}

type diagramShadow struct {
	box       scene.Rect
	transform scene.Matrix
	radius    float32
}

type diagramLabel struct {
	layout *ui.TextLayout
	color  ui.Color
	x, y   float32
	dx, dy float32
	scaleX float32
	scaleY float32
	angle  float32
}

const (
	mermaidSurface = "#FFFFFF"
	mermaidCard    = "#F2F2EE"
	mermaidInk     = "#2E3633"
)

// The renderer emits the edge label background as a path with this fill.
// Use a distinct color to identify it before the scene reaches the screen.
const edgeLabelFill = "#FE01FD"

func renderMermaid(source string) (image diagramImage, err error) {
	// Layout engines receive model-generated input. Leave the source visible
	// if the syntax, size or geometry is outside what the renderer can handle.
	defer func() {
		if recover() != nil {
			image, err = diagramImage{}, errors.New("Mermaid layout failed")
		}
	}()
	if len(source) > 12<<10 || strings.Count(source, "\n") > 160 {
		return image, errors.New("Mermaid source exceeds the preview limit")
	}
	fonts := text.NewFonts()
	_ = fonts.Add("Go", goregular.TTF)
	_ = fonts.Add("Go", gobold.TTF)
	fonts.Fallback("Go")
	drawing, err := mermaid.Render(source, mermaid.Options{Fonts: fonts, Config: map[string]any{
		"theme": "base", "layout": "dagre",
		"themeVariables": map[string]any{
			"background": mermaidSurface, "primaryColor": mermaidCard,
			"secondaryColor": "#EFF4F8", "tertiaryColor": "#F5F7F6",
			"mainBkg": mermaidCard, "nodeBorder": "#C6CAC6",
			"primaryTextColor": mermaidInk, "nodeTextColor": mermaidInk, "textColor": mermaidInk,
			"lineColor": "#6D7772", "edgeLabelBackground": edgeLabelFill,
			"erEdgeLabelBackground": edgeLabelFill, "requirementEdgeLabelBackground": edgeLabelFill,
			"radius": 12, "strokeWidth": 1.25,
		},
	}})
	if err != nil {
		return image, err
	}
	box := drawing.ViewBox
	if !(box.W > 0 && box.H > 0 && box.W <= 12000 && box.H <= 12000) {
		return image, errors.New("Mermaid diagram has invalid dimensions")
	}
	image.box = box
	image.stylePaths(drawing.Items, scene.Identity)
	image.labels = diagramLabels(drawing.Items, scene.Identity, 1, nil)
	// MyGo's SVG renderer draws the geometry. Its text engine draws the
	// labels with system font fallback, including Chinese and other scripts.
	image.shapes, err = ui.ParseSVG([]byte(drawing.SVG()))
	return image, err
}

func (image *diagramImage) stylePaths(items []scene.Item, transform scene.Matrix) {
	for _, item := range items {
		switch item := item.(type) {
		case *scene.Group:
			next := transform
			if !item.Transform.IsIdentity() {
				next = item.Transform.Then(transform)
			}
			image.stylePaths(item.Items, next)
		case *scene.Path:
			image.stylePath(item, transform)
		}
	}
}

func (image *diagramImage) stylePath(path *scene.Path, transform scene.Matrix) {
	box, rectangle := squareDiagramPath(path.Segs)
	if rectangle && path.Fill.R == 254 && path.Fill.G == 1 && path.Fill.B == 253 && path.Stroke.IsNone() {
		// Use the canvas color to break the line, leaving no visible badge.
		path.Fill = scene.RGB(255, 255, 255)
		path.Segs = scene.RectPath(box.X-5, box.Y, box.W+10, box.H, 7, 7)
		return
	}
	if path.Fill.IsNone() || path.Stroke.IsNone() {
		return
	}
	bounds := scene.Bounds(path.Segs)
	if bounds.W < 24 || bounds.H < 20 {
		return
	}
	switch {
	case rectangle:
		path.Fill = softenDiagramColor(path.Fill)
		path.Stroke = scene.None
		path.Segs = scene.RectPath(box.X, box.Y, box.W, box.H, 14, 14)
		image.shadows = append(image.shadows, diagramShadow{box: box, transform: transform, radius: 14})
	case isDiagramDiamond(path.Segs, bounds):
		path.Segs = roundDiagramDiamond(path.Segs)
		if path.Fill == scene.RGB(242, 242, 238) {
			path.Fill = scene.RGB(234, 236, 247)
		} else {
			path.Fill = softenDiagramColor(path.Fill)
		}
		path.Stroke = scene.None
		// A rotated square matches the shadow of an equal-sided diamond.
		if math.Abs(bounds.H-bounds.W) < .01 {
			side := bounds.W / math.Sqrt2
			cx, cy := bounds.X+bounds.W/2, bounds.Y+bounds.H/2
			placement := scene.Rotate(45).Then(scene.Translate(cx, cy)).Then(transform)
			image.shadows = append(image.shadows, diagramShadow{
				box: scene.Rect{X: -side / 2, Y: -side / 2, W: side, H: side}, transform: placement, radius: 6,
			})
		}
	}
}

func isDiagramDiamond(segs []scene.Seg, box scene.Rect) bool {
	if !isDiagramQuadrilateral(segs) {
		return false
	}
	for _, seg := range segs[:4] {
		point := seg.P[0]
		if math.Abs(point.X-box.X-box.W/2) > .01 && math.Abs(point.Y-box.Y-box.H/2) > .01 {
			return false
		}
	}
	return true
}

func roundDiagramDiamond(segs []scene.Seg) []scene.Seg {
	near := func(corner, neighbor scene.Pt) scene.Pt {
		dx, dy := neighbor.X-corner.X, neighbor.Y-corner.Y
		length := math.Hypot(dx, dy)
		fraction := min(9, length/3) / length
		return scene.Pt{X: corner.X + dx*fraction, Y: corner.Y + dy*fraction}
	}
	var path scene.PathBuilder
	for i, seg := range segs[:4] {
		corner := seg.P[0]
		before := near(corner, segs[(i+3)%4].P[0])
		after := near(corner, segs[(i+1)%4].P[0])
		if i == 0 {
			path.MoveTo(before.X, before.Y)
		} else {
			path.LineTo(before.X, before.Y)
		}
		path.QuadTo(corner.X, corner.Y, after.X, after.Y)
	}
	return path.Close().Segs
}

func softenDiagramColor(color scene.Color) scene.Color {
	chroma := max(color.R, color.G, color.B) - min(color.R, color.G, color.B)
	if chroma < 75 {
		return color
	}
	tone := ui.RGB(color.R, color.G, color.B).Mix(ui.Hex(mermaidSurface), .86)
	return scene.Color{R: tone.R, G: tone.G, B: tone.B, A: color.A}
}

func squareDiagramPath(segs []scene.Seg) (scene.Rect, bool) {
	if !isDiagramQuadrilateral(segs) {
		return scene.Rect{}, false
	}
	a, b, c, d := segs[0].P[0], segs[1].P[0], segs[2].P[0], segs[3].P[0]
	if a.X != d.X || b.X != c.X || a.Y != b.Y || c.Y != d.Y || b.X <= a.X || d.Y <= a.Y {
		return scene.Rect{}, false
	}
	return scene.Rect{X: a.X, Y: a.Y, W: b.X - a.X, H: d.Y - a.Y}, true
}

func isDiagramQuadrilateral(segs []scene.Seg) bool {
	if len(segs) != 5 || segs[0].Op != scene.MoveTo || segs[4].Op != scene.Close {
		return false
	}
	for _, seg := range segs[1:4] {
		if seg.Op != scene.LineTo {
			return false
		}
	}
	return true
}

func diagramLabels(items []scene.Item, transform scene.Matrix, opacity float64, labels []diagramLabel) []diagramLabel {
	for _, item := range items {
		switch item := item.(type) {
		case *scene.Group:
			next := transform
			if !item.Transform.IsIdentity() {
				next = item.Transform.Then(transform)
			}
			alpha := opacity
			if item.Opacity > 0 {
				alpha *= item.Opacity
			}
			labels = diagramLabels(item.Items, next, alpha, labels)
		case *scene.Text:
			if len(item.Runs) == 0 || !(item.Size > 0 && item.Size <= 256) {
				continue
			}
			x, y := transform.Apply(item.X, item.Y)
			label := diagramLabel{
				x: float32(x), y: float32(y),
				scaleX: float32(math.Hypot(transform.A, transform.B)),
				scaleY: float32(math.Hypot(transform.C, transform.D)),
				angle:  float32(item.Rotate + math.Atan2(transform.B, transform.A)*180/math.Pi),
			}
			if !finiteLabel(label) {
				continue
			}
			spans := make([]ui.Span, 0, len(item.Runs))
			for _, run := range item.Runs {
				color := item.Color
				if !run.Color.IsNone() {
					color = run.Color
				}
				spans = append(spans, ui.Span{
					Text: run.Text, Size: float32(item.Size), Weight: run.Weight,
					Italic: run.Italic, Underline: run.Underline,
					Color: ui.RGBA(color.R, color.G, color.B, float32(color.A*opacity)),
				})
			}
			label.layout = ui.ShapeRichText(spans, ui.Font{Size: float32(item.Size)}, 0)
			label.color = spans[0].Color
			lines := label.layout.Lines()
			if len(lines) == 0 {
				continue
			}
			line := lines[0]
			label.dy = -line.Baseline
			switch item.Anchor {
			case scene.Middle:
				label.dx = -line.Rect.W / 2
			case scene.End:
				label.dx = -line.Rect.W
			}
			switch item.Baseline {
			case scene.Central:
				label.dy += (line.Ascent - line.Descent) / 2
			case scene.MiddleBaseline:
				label.dy += float32(item.Size) * .25
			case scene.Hanging:
				label.dy += line.Ascent * .8
			}
			labels = append(labels, label)
		}
	}
	return labels
}

func finiteLabel(label diagramLabel) bool {
	for _, v := range []float32{label.x, label.y, label.angle, label.scaleX, label.scaleY} {
		if !(v >= -1e6 && v <= 1e6) {
			return false
		}
	}
	return label.scaleX > 0 && label.scaleY > 0
}

func (image diagramImage) draw(p *ui.Painter, rect ui.Rect) {
	if rect.W <= 0 || rect.H <= 0 {
		return
	}
	w, h := float32(image.box.W), float32(image.box.H)
	scale := min(rect.W/w, rect.H/h)
	left := rect.X + (rect.W-w*scale)/2
	top := rect.Y + (rect.H-h*scale)/2
	for _, shadow := range image.shadows {
		box := shadow.box
		centerX, centerY := shadow.transform.Apply(box.X+box.W/2, box.Y+box.H/2)
		x := left + float32(centerX-image.box.X)*scale
		y := top + float32(centerY-image.box.Y)*scale
		width := float32(box.W*math.Hypot(shadow.transform.A, shadow.transform.B)) * scale
		height := float32(box.H*math.Hypot(shadow.transform.C, shadow.transform.D)) * scale
		angle := float32(math.Atan2(shadow.transform.B, shadow.transform.A) * 180 / math.Pi)
		if !(width > 0 && width <= 12000 && height > 0 && height <= 12000 &&
			x >= -1e6 && x <= 1e6 && y >= -1e6 && y <= 1e6 && angle >= -180 && angle <= 180) {
			continue
		}
		p.Transform(ui.Rotation(angle), x, y, func() {
			// Keep the shadow's cutout inside the SVG fill. The native
			// shadow and SVG edge can round to different device pixels.
			inset := min(float32(1.5), min(width, height)/4)
			p.Shadow(ui.Rect{X: x - width/2 + inset, Y: y - height/2 + inset, W: width - 2*inset, H: height - 2*inset},
				max(0, shadow.radius*scale-inset), 0, 2*scale, 5*scale, inset, ui.RGBA(20, 23, 22, .20))
		})
	}
	p.Image(image.shapes, ui.Rect{X: left, Y: top, W: w * scale, H: h * scale}, ui.Contain)
	for _, label := range image.labels {
		x := left + (label.x-float32(image.box.X))*scale
		y := top + (label.y-float32(image.box.Y))*scale
		p.Transform(ui.Scaling(label.scaleX*scale, label.scaleY*scale).Then(ui.Rotation(label.angle)), x, y, func() {
			p.TextLayout(label.layout, x+label.dx, y+label.dy, label.color)
		})
	}
}

type mermaidRender struct {
	image diagramImage
	ready bool
}

func mermaidBlock() func(*ui.Context, *markdown.CodeBlockView) {
	var mu sync.Mutex
	cache := make(map[string]*mermaidRender)
	workers := make(chan struct{}, 2)
	return func(c *ui.Context, block *markdown.CodeBlockView) {
		if block.Open || len(block.Code()) > 12<<10 {
			markdown.DefaultCodeBlock(c, block)
			return
		}
		source := block.Code()
		mu.Lock()
		result := cache[source]
		if result == nil {
			select {
			case workers <- struct{}{}:
				result = &mermaidRender{}
				if len(cache) >= 64 {
					for key, entry := range cache {
						if entry.ready {
							delete(cache, key)
							break
						}
					}
				}
				cache[source] = result
				services := c.Services()
				go func(result *mermaidRender) {
					image, _ := renderMermaid(source)
					mu.Lock()
					result.image, result.ready = image, true
					<-workers
					mu.Unlock()
					services.Invalidate()
				}(result)
			default:
			}
		}
		var image diagramImage
		if result != nil {
			image = result.image
		}
		mu.Unlock()
		if image.shapes == nil {
			markdown.DefaultCodeBlock(c, block)
			return
		}
		theme := markdown.DefaultTheme(c.Theme())
		ui.Column(c).FillWidth().MinWidth(0).Radius(16).
			Border(1, ui.Hex("#E2E9E5")).Background(ui.Hex(mermaidSurface)).Clip().Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Padding(8, 14).
				Background(ui.Hex("#F8FAF9")).Children(func() {
				ui.Text(c, "Mermaid").FontSize(12).TextColor(theme.Muted)
				block.CopyButton(c)
			})
			w, h := float32(image.box.W), float32(image.box.H)
			ui.Box(c).Width(w).MaxWidthPercent(100).MaxHeight(min(560, h)).AspectRatio(w/h).
				AlignSelf(ui.Center).Margin(6, 0, 10).Role(ui.RoleImage).Label("Mermaid").Draw(image.draw)
		})
	}
}
