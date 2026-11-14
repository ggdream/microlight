package theme

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var MenuTheme = func() *ui.Theme {
	t := ui.LightTheme()
	t.Background, t.Border, t.Text, t.TextMuted = Side, Border, Ink, Muted
	t.Accent, t.AccentText, t.Danger = Tint, Ink, ErrorFG
	return t
}()

type PopupItem struct {
	Label    string
	Icon     *mygo.MenuIcon
	Disabled bool
	OnSelect func()
}

// PopupMenu opens an icon-and-text menu above its trigger.
func PopupMenu(c *ui.Context, trigger func(*ui.Context) ui.Element, items ...PopupItem) {
	ui.Themed(c, MenuTheme).Background(ui.Transparent).Shrink(0).Children(func() {
		trigger(c.Key("trigger")).PopoverMenuButton(func(menu *ui.Menu) {
			for _, item := range items {
				if menu.Item(item.Label).Icon(item.Icon).Disabled(item.Disabled).Chosen() && item.OnSelect != nil {
					item.OnSelect()
				}
			}
		}, ui.PopoverMenuButtonOptions{At: ui.AnchorTopStart, Self: ui.AnchorBottomStart})
	})
}
