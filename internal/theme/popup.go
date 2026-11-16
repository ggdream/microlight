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
			popupItems(menu, items)
		}, ui.PopoverMenuButtonOptions{At: ui.AnchorTopStart, Self: ui.AnchorBottomStart})
	})
}

// PopupContextMenu attaches the shared popup items to a right-click target.
// Build the target in MenuTheme, as for the application's other popups.
func PopupContextMenu(target ui.Element, items ...PopupItem) {
	target.PopoverMenu(func(menu *ui.Menu) { popupItems(menu, items) })
}

func popupItems(menu *ui.Menu, items []PopupItem) {
	for _, item := range items {
		if menu.Item(item.Label).Icon(item.Icon).Disabled(item.Disabled).Chosen() && item.OnSelect != nil {
			item.OnSelect()
		}
	}
}
