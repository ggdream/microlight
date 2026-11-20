package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/internal/desktop"
	"github.com/ggdream/microlight/pkg/viewport"
)

func windowLayout(c *ui.Context) viewport.Layout {
	return viewport.Available(c)
}

func touchControls(c *ui.Context) bool { return desktop.Mobile || windowLayout(c).Compact() }

// The composer also needs to fit when side panels consume part of a wide window.
func (app *chatApp) composerLayout(c *ui.Context) viewport.Layout {
	layout := windowLayout(c)
	if !layout.Compact() {
		layout.Width -= app.sidebarWidth
		if layout.Width > 0 && windowLayout(c).Expanded() && app.toolPanel.visible {
			layout.Width -= previewPaneWidth(layout.Width)
		}
	}
	return layout
}

func previewPaneWidth(contentWidth float32) float32 {
	return min(680, contentWidth*.47, max(180, contentWidth-300))
}

func composerControlHeight(layout viewport.Layout, desktopHeight float32) float32 {
	return layout.Density(layout.Y(desktopHeight), 34)
}

// popoverHeight bounds anchored content by the larger space beside the anchor.
// Before the first layout, use the window height and let MyGo place the panel.
func popoverHeight(c *ui.Context, anchor ui.Element, preferred float32) float32 {
	layout := windowLayout(c)
	bounds := anchor.LayoutBounds()
	if bounds.H <= 0 {
		return layout.FitHeight(preferred, 8)
	}
	top, _, _, _ := c.SafeArea()
	space := max(bounds.Y-top, top+layout.Height-bounds.Y-bounds.H)
	return min(preferred, max(0, space-12))
}

func layoutMessageFont(c *ui.Context) float32 { return windowLayout(c).Text(16) }
