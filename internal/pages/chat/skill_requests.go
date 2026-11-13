package chat

import (
	"context"
	"slices"
	"strings"

	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/internal/skills"
	"github.com/ggdream/microlight/internal/workspace"
)

func withRequestedSkills(ctx context.Context, catalog *skills.Catalog, text string) context.Context {
	var names []string
	for _, token := range strings.Fields(text) {
		name, slash := strings.CutPrefix(token, "/")
		if slash && catalog.Has(name) && !slices.Contains(names, name) &&
			composer.ValidSkillName(name) {
			names = append(names, name)
		}
	}
	return workspace.WithRequestedSkills(ctx, names)
}
