package settings

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/localtools"
)

func BrowserModeSelect(c *ui.Context, icons asseticon.Set, mode *localtools.BrowserMode, change func()) {
	settingsSelect(c, icons, locales.BrowserMode(), mode,
		[]localtools.BrowserMode{localtools.BrowserForeground, localtools.BrowserBackground}, func(mode localtools.BrowserMode) string {
			if mode == localtools.BrowserBackground {
				return locales.BrowserModeBackground()
			}
			return locales.BrowserModeForeground()
		}, change)
}
