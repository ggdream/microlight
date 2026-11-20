package theme

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
	"github.com/ggdream/microlight/pkg/viewport"
)

var MenuTheme = func() *ui.Theme {
	t := material3.Monochrome.Theme()
	t.Background = material3.Monochrome.SurfaceLow
	t.Border = material3.DividerColor()
	t.Accent, t.AccentText, t.Danger = material3.Monochrome.SurfaceLow.Mix(Ink, .04), Ink, ErrorFG
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
			// Replace MyGo's 180-point minimum with intrinsic content width,
			// capped by the available window size.
			material3.Menu(panel).MaxWidth(viewport.Available(c).FitWidth(480, 8)).Padding(6).Gap(2)
			if panelStyle != nil {
				panelStyle(panel)
			}
		}
		// MyGo invokes the row style in display order for this flat menu.
		placement.Item = func(row ui.Element) {
			styleMenuItem(row)
			if itemStyle != nil {
				itemStyle(row)
			}
			if rowIndex < len(items) && items[rowIndex].Selected {
				row.AccessSelected(true).Background(Selected).TextColor(Accent).Radius(7)
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
	target.PopoverMenu(func(menu *ui.Menu) { popupItems(menu, items) }, ContextMenuStyle())
}

func ContextMenuStyle() ui.PopoverMenuOptions {
	return ui.PopoverMenuOptions{Panel: func(panel ui.Element) { material3.Menu(panel) }, Item: styleMenuItem}
}

func MenuButtonStyle(options ui.PopoverMenuButtonOptions) ui.PopoverMenuButtonOptions {
	options.Panel = func(panel ui.Element) { material3.Menu(panel) }
	options.Item = styleMenuItem
	return options
}

func styleMenuItem(item ui.Element) {
	// Give the trailing side more breathing room without widening the icon
	// gutter. PaddingEnd also mirrors correctly for right-to-left menus.
	item.Radius(material3.ShapeSmall).PaddingEnd(16)
	if item.Highlighted() || item.Hovered() {
		item.Background(MenuTheme.Accent).TextColor(Ink)
	}
}

// MenuSurface scopes the menu's highlight foreground, including template icons,
// without changing filled buttons or checkboxes elsewhere in the window.
func MenuSurface(c *ui.Context) ui.Element {
	return ui.Themed(c, PopupTheme(c)).FillWidth().Background(ui.Transparent)
}

func popupItems(menu *ui.Menu, items []PopupItem) {
	for _, item := range items {
		entry := menu.Item(item.Label).Icon(item.Icon).Disabled(item.Disabled)
		if entry.Chosen() && item.OnSelect != nil {
			item.OnSelect()
		}
	}
}
