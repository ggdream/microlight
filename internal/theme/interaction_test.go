package theme

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/material3"
)

func TestNestedRowTargetDoesNotAddAnotherHoverLayer(t *testing.T) {
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(White).Children(func() {
			row := ui.Row(c).Width(240).Height(40)
			if row.Hovered() {
				row.Background(Hover)
			}
			row.Children(func() { PointerButton(c, false).Label("Conversation").Width(180).FillHeight() })
		})
	}, 260, 80)
	tester.Move(90, 20)
	img := tester.Image()
	if inner, outer := img.RGBAAt(90, 20), img.RGBAAt(220, 20); inner != outer {
		t.Fatalf("nested target darkened the row twice: inner %v, outer %v", inner, outer)
	}
}

func TestHighlightedMenuTemplateIconRemainsDark(t *testing.T) {
	svg, err := ui.ParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M0 0h16v16H0z"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	icon, err := mygo.NewMenuIconSVG(svg, true)
	if err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(material3.Monochrome.Theme())
		MenuSurface(c).Children(func() {
			PointerButton(c, false).Label("Target").Size(100, 30).PopoverMenu(func(m *ui.Menu) {
				m.Item("Pinned").Icon(icon)
			}, ContextMenuStyle())
		})
	}, 400, 200)
	if err := tester.RightClick("Target"); err != nil {
		t.Fatal(err)
	}
	row, ok := tester.Find("Pinned")
	if !ok {
		t.Fatal("menu item missing")
	}
	countDark := func() int {
		img := tester.Image()
		count := 0
		for y := int(row.Y + row.H/2 - 5); y < int(row.Y+row.H/2+5); y++ {
			for x := int(row.X + 7); x < int(row.X+23); x++ {
				if img.RGBAAt(x, y).R < 100 {
					count++
				}
			}
		}
		return count
	}
	before := countDark()
	tester.Move(row.X+row.W/2, row.Y+row.H/2)
	if after := countDark(); before < 10 || after < before/2 {
		t.Fatalf("hover lost the dark menu icon: dark pixels %d -> %d", before, after)
	}
}

func TestMenuSeparatorRemainsVisibleWithoutPanelOutline(t *testing.T) {
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(material3.Monochrome.Theme())
		MenuSurface(c).Children(func() {
			PointerButton(c, false).Label("Target").Size(100, 30).PopoverMenu(func(m *ui.Menu) {
				m.Item("Pin")
				m.Separator()
				m.Item("Archive")
			}, ContextMenuStyle())
		})
	}, 400, 200)
	if err := tester.RightClick("Target"); err != nil {
		t.Fatal(err)
	}
	a, okA := tester.AccessByRole(ui.RoleMenuItem, "Pin")
	b, okB := tester.AccessByRole(ui.RoleMenuItem, "Archive")
	if !okA || !okB {
		t.Fatal("menu groups missing")
	}
	y := int((a.Bounds.Y + a.Bounds.H + b.Bounds.Y) / 2)
	x := int(a.Bounds.X + a.Bounds.W/2)
	visible := false
	for dy := -2; dy <= 2; dy++ {
		visible = visible || tester.Image().RGBAAt(x, y+dy).R < 230
	}
	if !visible {
		t.Fatal("menu separator inherited the transparent control-border color")
	}
}
