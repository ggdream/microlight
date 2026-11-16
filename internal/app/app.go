// Package app starts the desktop application and wires its main page.
package app

import (
	"io/fs"

	"github.com/ggdream/microlight/internal/pages/chat"

	"github.com/egoist/mygo"
)

func Run(assets fs.FS) {
	// Keep the existing data directory and single-instance lock across renames.
	// The chat page sets the localized display name after opening the store.
	mygo.App.SetName("微光")
	if mygo.App.RequestSingleInstanceLock() {
		chat.Run(assets)
	}
}
