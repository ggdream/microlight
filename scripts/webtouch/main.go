// Prepare a temporary build copy of the pinned MyGo browser touch adapter.
// The module cache stays intact. Remove this when upstream sets SurfaceEvent.Touch.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	module := flag.String("module", "", "MyGo module directory")
	out := flag.String("out", "", "temporary build directory")
	flag.Parse()
	if err := generate(*module, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(module, out string) error {
	source := filepath.Join(module, "internal", "web", "surface.go")
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	const marker = "func (s *surface) send(ev platform.SurfaceEvent) bool {"
	if strings.Count(string(data), marker) != 1 {
		return fmt.Errorf("unsupported MyGo browser adapter")
	}
	patched := strings.Replace(string(data), marker, marker+"\n\tev.Touch = ev.Pointer == platform.PointerTouch", 1)
	if err := os.MkdirAll(out, 0700); err != nil {
		return err
	}
	copyDir, err := filepath.Abs(filepath.Join(out, "mygo"))
	if err != nil {
		return err
	}
	if err := os.CopyFS(copyDir, os.DirFS(module)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(copyDir, "internal", "web", "surface.go"), []byte(patched), 0600); err != nil {
		return err
	}
	file, err := filepath.Abs(filepath.Join(out, "go.mod"))
	if err != nil {
		return err
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, name), data, 0600); err != nil {
			return err
		}
	}
	cmd := exec.Command("go", "mod", "edit", "-modfile="+file, "-replace=github.com/egoist/mygo="+copyDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("prepare web module: %s: %w", output, err)
	}
	fmt.Println(file)
	return nil
}
