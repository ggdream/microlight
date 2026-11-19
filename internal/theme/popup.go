package theme

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/viewport"
)

var MenuTheme = func() *ui.Theme {
	t := ui.LightTheme()
	t.Background, t.Border, t.Text, t.TextMuted = Side, Border, Ink, Muted
	t.Accent, t.AccentText, t.Danger = Tint, Ink, ErrorFG
	return t
}()

func PopupTheme(c *ui.Context) *ui.Theme {
	t := *MenuTheme
	t.FontSize = viewport.Font(c, t.FontSize)
	return &t
}

type PopupItem struct {
	Label    string
	Icon     *mygo.MenuIcon
	Disabled bool
	Selected bool
	OnSelect func()
}

// PopupMenu opens an icon-and-text menu above its trigger.
func PopupMenu(c *ui.Context, trigger func(*ui.Context) ui.Element, items ...PopupItem) {
	PopupMenuAt(c, trigger, ui.PopoverMenuButtonOptions{At: ui.AnchorTopStart, Self: ui.AnchorBottomStart}, items...)
}

// PopupMenuAt opens the shared popup with the specified placement.
func PopupMenuAt(c *ui.Context, trigger func(*ui.Context) ui.Element, placement ui.PopoverMenuButtonOptions, items ...PopupItem) {
	ui.Themed(c, PopupTheme(c)).Background(ui.Transparent).Shrink(0).Children(func() {
		var rowIndex int
		panelStyle, itemStyle := placement.Panel, placement.Item
		placement.Panel = func(panel ui.Element) {
			rowIndex = 0
			if panelStyle != nil {
				panelStyle(panel)
			}
		}
		// MyGo invokes the row style in display order for this flat menu.
		placement.Item = func(row ui.Element) {
			if itemStyle != nil {
				itemStyle(row)
			}
			if rowIndex < len(items) && items[rowIndex].Selected {
				row.AccessSelected(true).Background(Tint).TextColor(Accent).Radius(7)
			}
			rowIndex++
		}
		trigger(c.Key("trigger")).PopoverMenuButton(func(menu *ui.Menu) {
			popupItems(menu, items)
		}, placement)
	})
}

// PopupContextMenu attaches the shared popup items to a right-click target.
// Build the target in MenuTheme, as for the application's other popups.
func PopupContextMenu(target ui.Element, items ...PopupItem) {
	target.PopoverMenu(func(menu *ui.Menu) { popupItems(menu, items) })
}

func popupItems(menu *ui.Menu, items []PopupItem) {
	for _, item := range items {
		entry := menu.Item(item.Label).Icon(item.Icon).Disabled(item.Disabled)
		if entry.Chosen() && item.OnSelect != nil {
			item.OnSelect()
		}
	}
}
