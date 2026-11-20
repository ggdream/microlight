package material3

import (
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/viewport"
)

type TextRole uint8

const (
	DisplaySmall TextRole = iota
	HeadlineMedium
	TitleLarge
	TitleMedium
	BodyLarge
	BodyMedium
	BodySmall
	LabelLarge
	LabelMedium
	LabelSmall
)

func Text(c *ui.Context, text string, role TextRole) ui.Element {
	size, weight := float32(14), 400
	switch role {
	case DisplaySmall:
		size = 36
	case HeadlineMedium:
		size = 28
	case TitleLarge:
		size = 22
	case TitleMedium:
		size, weight = 16, 500
	case BodyLarge:
		size = 16
	case BodyMedium:
		size = 14
	case BodySmall:
		size = 12
	case LabelLarge:
		size, weight = 14, 500
	case LabelMedium:
		size, weight = 12, 500
	case LabelSmall:
		size, weight = 11, 500
	}
	return ui.Text(c, text).FontSize(viewport.Font(c, size)).FontWeight(weight).TextColor(Monochrome.OnSurface)
}
