package viewport

import "github.com/egoist/mygo/ui"

// Available reads the window's logical layout area, after MyGo's insets.
func Available(c *ui.Context) Layout {
	width, height := c.Size()
	top, right, bottom, left := c.SafeArea()
	return Layout{Width: max(0, width-left-right), Height: max(0, height-top-bottom)}
}

// Font applies the window's typography scale to an explicit size.
func Font(c *ui.Context, points float32) float32 {
	return Available(c).Text(points)
}
