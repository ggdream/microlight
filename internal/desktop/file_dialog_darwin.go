//go:build darwin && !ios

package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// MyGo's macOS picker unions its filters. Use an AppKit accessory selector so
// an All files option does not disable the initial extension filter. This is
// a Go/purego bridge; it requires no native module or cgo build step.
type fileTypePanel struct {
	panel objc.ID
	types []objc.ID
}

// Accessed only on the AppKit main thread, including Objective-C callbacks.
var fileTypePanels = map[objc.ID]fileTypePanel{}

var fileTypeController = sync.OnceValues(func() (objc.Class, error) {
	if _, err := purego.Dlopen("/System/Library/Frameworks/UniformTypeIdentifiers.framework/UniformTypeIdentifiers", purego.RTLD_NOW|purego.RTLD_LOCAL); err != nil {
		return 0, err
	}
	return objc.RegisterClass("MicrolightFileTypeController", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{
		Cmd: objc.RegisterName("selectFileType:"),
		Fn: func(self objc.ID, _ objc.SEL, sender objc.ID) {
			state, ok := fileTypePanels[self]
			index := int(sender.Send(objc.RegisterName("indexOfSelectedItem")))
			if ok && index >= 0 && index < len(state.types) {
				state.panel.Send(objc.RegisterName("setAllowedContentTypes:"), state.types[index])
			}
		},
	}})
})

func dialogString(value string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), value)
}

// OpenFilteredFiles opens a modal native file picker. Call it from a worker,
// just like mygo.Dialog.Open; all AppKit work stays on the UI thread.
func OpenFilteredFiles(options mygo.OpenDialogOptions) (paths []string, err error) {
	if len(options.Filters) < 2 {
		return mygo.Dialog.Open(options)
	}
	mygo.RunOnMain(func() {
		pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
		defer pool.Send(objc.RegisterName("drain"))
		controllerClass, initErr := fileTypeController()
		if initErr != nil {
			err = initErr
			return
		}
		panel := objc.ID(objc.GetClass("NSOpenPanel")).Send(objc.RegisterName("openPanel"))
		panel.Send(objc.RegisterName("setTitle:"), dialogString(options.Title))
		panel.Send(objc.RegisterName("setCanChooseFiles:"), true)
		panel.Send(objc.RegisterName("setCanChooseDirectories:"), false)
		panel.Send(objc.RegisterName("setAllowsMultipleSelection:"), options.Multiple)
		panel.Send(objc.RegisterName("setShowsHiddenFiles:"), options.ShowHiddenFiles)
		if options.DefaultPath != "" {
			dir := options.DefaultPath
			if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
				dir = filepath.Dir(dir)
			}
			url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), dialogString(dir))
			panel.Send(objc.RegisterName("setDirectoryURL:"), url)
		}
		controller := objc.ID(controllerClass).Send(objc.RegisterName("new"))
		selector := objc.ID(objc.GetClass("NSPopUpButton")).Send(objc.RegisterName("new"))
		defer func() {
			selector.Send(objc.RegisterName("setTarget:"), objc.ID(0))
			panel.Send(objc.RegisterName("setAccessoryView:"), objc.ID(0))
			delete(fileTypePanels, controller)
			selector.Send(objc.RegisterName("release"))
			controller.Send(objc.RegisterName("release"))
		}()
		state := fileTypePanel{panel: panel}
		ut := objc.ID(objc.GetClass("UTType"))
		for _, filter := range options.Filters {
			types := objc.ID(objc.GetClass("NSMutableArray")).Send(objc.RegisterName("array"))
			for _, extension := range filter.Extensions {
				var kind objc.ID
				if extension == "*" {
					kind = ut.Send(objc.RegisterName("typeWithIdentifier:"), dialogString("public.item"))
				} else {
					dataType := ut.Send(objc.RegisterName("typeWithIdentifier:"), dialogString("public.data"))
					kind = ut.Send(objc.RegisterName("typeWithFilenameExtension:conformingToType:"), dialogString(strings.TrimPrefix(extension, ".")), dataType)
				}
				if kind != 0 {
					types.Send(objc.RegisterName("addObject:"), kind)
				}
			}
			if types.Send(objc.RegisterName("count")) == 0 {
				err = errors.New("file filter requires a supported extension")
				return
			}
			state.types = append(state.types, types)
			selector.Send(objc.RegisterName("addItemWithTitle:"), dialogString(filter.Name))
		}
		fileTypePanels[controller] = state
		selector.Send(objc.RegisterName("setTarget:"), controller)
		selector.Send(objc.RegisterName("setAction:"), objc.RegisterName("selectFileType:"))
		selector.Send(objc.RegisterName("selectItemAtIndex:"), 0)
		selector.Send(objc.RegisterName("sizeToFit"))
		panel.Send(objc.RegisterName("setAccessoryView:"), selector)
		panel.Send(objc.RegisterName("setAccessoryViewDisclosed:"), true)
		panel.Send(objc.RegisterName("setAllowedContentTypes:"), state.types[0])
		objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication")).Send(objc.RegisterName("activateIgnoringOtherApps:"), true)
		if panel.Send(objc.RegisterName("runModal")) != 1 {
			return
		}
		urls := panel.Send(objc.RegisterName("URLs"))
		for i, count := 0, int(urls.Send(objc.RegisterName("count"))); i < count; i++ {
			url := urls.Send(objc.RegisterName("objectAtIndex:"), i)
			path := url.Send(objc.RegisterName("path"))
			paths = append(paths, objc.Send[string](path, objc.RegisterName("UTF8String")))
		}
	})
	return paths, err
}
