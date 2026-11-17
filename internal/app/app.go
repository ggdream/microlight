// Package app starts the desktop application and wires its main page.
package app

import (
	"io/fs"

	"github.com/ggdream/microlight/internal/pages/chat"

	"github.com/egoist/mygo"
)

func Run(assets fs.FS) {
	// Keep the single-instance lock shared with earlier releases.
	// The chat page sets the data path and localized display name separately.
	mygo.App.SetName("微光")
	if mygo.App.RequestSingleInstanceLock() {
		chat.Run(assets)
	}
}
