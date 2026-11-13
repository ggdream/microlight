package chat

import (
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/pages/settings"

	"github.com/egoist/mygo/ui"
)

type extensionsForm struct {
	mcp                       settings.MCPForm
	skillIndex                *skillIndex
	skillQuery, expandedSkill string
}

func (app *chatApp) mcpSettings(c *ui.Context) {
	app.extensionsForm.mcp.View(c, icons, app.settings.MCPServers, func(servers []mcptools.Server) error {
		config := app.settings
		config.MCPServers = servers
		return app.applySettings(config)
	})
}
