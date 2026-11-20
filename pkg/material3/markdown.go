package material3

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// MarkdownTheme keeps content rules and table separators legible even though
// the host's control-border color is transparent.
func MarkdownTheme(theme *ui.Theme) *markdown.Theme {
	t := markdown.DefaultTheme(theme)
	t.CodeBackground, t.CodeBlockBackground = Monochrome.SurfaceContainer, Monochrome.SurfaceContainer
	t.CodeBlockBorder, t.CodeBlockRadius = ui.Transparent, ShapeMedium
	t.TableHeader, t.TableStripe = Monochrome.SurfaceContainer, Monochrome.SurfaceLow
	t.TableBorder, t.Rule = Monochrome.OutlineVariant.Alpha(.65), Monochrome.OutlineVariant.Alpha(.65)
	t.QuoteBar = Monochrome.OnSurfaceVariant.Alpha(.45)
	return t
}
