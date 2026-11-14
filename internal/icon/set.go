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

// Load reads the embedded Fluent UI icons and GitHub mark.
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
		if err := set.loadSVG(assets, name, "assets/fluent/"+file.Name()); err != nil {
			return Set{}, err
		}
	}
	if err := set.loadSVG(assets, "github", "assets/octicons/mark-github-16.svg"); err != nil {
		return Set{}, err
	}
	for _, name := range []string{"edit", "pin", "pin_off", "archive", "delete", "document_text", "skill", "update_tasks", "copy", "panel_left_expand", "desktop", "cube", "desktop_signal"} {
		menu, err := mygo.NewMenuIconSVG(set.svg[name], true)
		if err != nil {
			return Set{}, err
		}
		set.menu[name] = menu
	}
	return set, nil
}

func (s Set) loadSVG(assets fs.FS, name, path string) error {
	data, err := fs.ReadFile(assets, path)
	if err != nil {
		return err
	}
	svg, err := ui.ParseSVG(data)
	if err != nil {
		return err
	}
	s.svg[name] = svg
	return nil
}

func (s Set) SVG(name string) *ui.SVG         { return s.svg[name] }
func (s Set) Menu(name string) *mygo.MenuIcon { return s.menu[name] }
