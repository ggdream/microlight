// Package app starts the desktop application and wires its main page.
package app

import (
	"io/fs"

	"github.com/ggdream/microlight/internal/pages/chat"
	"github.com/ggdream/microlight/internal/workspace"

	"github.com/egoist/mygo"
)

func Run(assets fs.FS) {
	mygo.App.SetName("微光")
	if mygo.App.RequestSingleInstanceLock() {
		chat.Run(assets, workspace.Open)
	}
}
