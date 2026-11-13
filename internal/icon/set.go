package icon

import (
	"io/fs"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type Set struct {
	svg  map[string]*ui.SVG
	menu map[string]*mygo.MenuIcon
}

// Load reads Microsoft Fluent UI System Icons from an embedded directory.
func Load(assets fs.FS) (Set, error) {
	set := Set{svg: make(map[string]*ui.SVG), menu: make(map[string]*mygo.MenuIcon)}
	files, err := fs.ReadDir(assets, "assets/fluent")
	if err != nil {
		return Set{}, err
	}
	for _, file := range files {
		name, ok := strings.CutSuffix(file.Name(), ".svg")
		if !ok || file.IsDir() {
			continue
		}
		data, err := fs.ReadFile(assets, "assets/fluent/"+file.Name())
		if err != nil {
			return Set{}, err
		}
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return Set{}, err
		}
		set.svg[name] = svg
	}
	for _, name := range []string{"edit", "pin", "pin_off", "archive", "delete", "document_text", "skill", "update_tasks", "copy", "panel_left_expand"} {
		menu, err := mygo.NewMenuIconSVG(set.svg[name], true)
		if err != nil {
			return Set{}, err
		}
		set.menu[name] = menu
	}
	return set, nil
}

func (s Set) SVG(name string) *ui.SVG         { return s.svg[name] }
func (s Set) Menu(name string) *mygo.MenuIcon { return s.menu[name] }
