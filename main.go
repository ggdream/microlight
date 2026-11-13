package main

import (
	"embed"

	"github.com/ggdream/microlight/internal/app"
)

//go:embed assets
var assets embed.FS

func main() {
	app.Run(assets)
}
