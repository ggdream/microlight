// Package markup configures MyGo Markdown with LaTeX formula rendering.
package markup

import (
	"log"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	texmath "github.com/go-tex/math"
)

type formulaKey struct {
	source  string
	size    int
	display bool
}

type renderedFormula struct {
	svg *ui.SVG
	texmath.Metrics
}

func New(ink ui.Color) *markdown.Options {
	options := &markdown.Options{WrapCode: true, Extensions: parser.Default | parser.ExtDollarMath}
	renderer, err := texmath.New(texmath.DefaultFont())
	if err != nil {
		log.Printf("初始化公式渲染失败：%v", err)
		return options
	}
	// MyGo asks for a formula again as its paragraph streams in.
	cache := make(map[formulaKey]renderedFormula)
	formula := func(key formulaKey) renderedFormula {
		if cached, ok := cache[key]; ok {
			return cached
		}
		if len(key.source) > 4096 {
			return renderedFormula{}
		}
		render := renderer.RenderSVGMetrics
		if key.display {
			render = renderer.RenderDisplaySVGMetrics
		}
		source, metrics, err := render(key.source, key.size)
		var result renderedFormula
		if err == nil && len(source) <= 256<<10 &&
			metrics.Width > 0 && metrics.Width <= 1800 &&
			metrics.Height+metrics.Depth > 0 && metrics.Height+metrics.Depth <= 1200 {
			result.svg, _ = ui.ParseSVG([]byte(source))
			result.Metrics = metrics
		}
		if len(cache) >= 256 {
			clear(cache)
		}
		cache[key] = result
		return result
	}
	options.InlineMath = func(source string, size float32) (markdown.InlineObject, bool) {
		f := formula(formulaKey{source: source, size: int(math.Round(float64(size)))})
		if f.svg == nil {
			return markdown.InlineObject{}, false
		}
		return markdown.InlineObject{
			Width: float32(f.Width), Height: float32(f.Height), Depth: float32(f.Depth),
			Draw: func(p *ui.Painter, r ui.Rect) { p.Icon(f.svg, r, ink) },
		}, true
	}
	options.Math = func(c *ui.Context, source string) {
		f := formula(formulaKey{source: source, size: 17, display: true})
		ui.ScrollHorizontal(c).AlignSelf(ui.Stretch).Children(func() {
			if f.svg == nil {
				ui.Text(c, source).Font("monospace").NoWrap().Selectable().Margin(0, ui.Auto)
				return
			}
			ui.Icon(c, f.svg).Size(float32(f.Width), float32(f.Height+f.Depth)).Shrink(0).
				TextColor(ink).Label(source).Margin(2, ui.Auto)
		})
	}
	return options
}
