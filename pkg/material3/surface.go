package material3

import "github.com/egoist/mygo/ui"

type CardVariant uint8

const DialogRadius float32 = 20

const (
	ElevatedCard CardVariant = iota
	FilledCard
)

// Elevate replaces prior shadows with Flutter's three-layer elevation model.
// Short-blur contact layers define the edge; ambient shadow softens the falloff.
func Elevate(element ui.Element, level int) ui.Element {
	element.NoShadow()
	var y, blur, spread, penumbraY, penumbraBlur, ambientBlur float32
	switch level {
	case 0:
		return element
	case 1:
		y, blur, spread, penumbraY, penumbraBlur, ambientBlur = 2, 1, -1, 1, 1, 3
	case 2:
		y, blur, spread, penumbraY, penumbraBlur, ambientBlur = 3, 1, -2, 2, 2, 5
	case 3:
		y, blur, spread, penumbraY, penumbraBlur, ambientBlur = 3, 3, -2, 3, 4, 8
	case 4:
		y, blur, spread, penumbraY, penumbraBlur, ambientBlur = 2, 4, -1, 4, 5, 10
	case 5:
		y, blur, spread, penumbraY, penumbraBlur, ambientBlur = 3, 5, -1, 6, 10, 18
	default:
		return element
	}
	return element.Shadow(0, y, blur, spread, Monochrome.Shadow.Alpha(.20)).
		Shadow(0, penumbraY, penumbraBlur, 0, Monochrome.Shadow.Alpha(.14)).
		Shadow(0, 1, ambientBlur, 0, Monochrome.Shadow.Alpha(.12))
}

func Card(c *ui.Context, variant CardVariant) ui.Element {
	return CardStyle(ui.Column(c).FillWidth(), variant)
}

func CardStyle(element ui.Element, variant CardVariant) ui.Element {
	element.BorderWidth(0).Radius(ShapeMedium)
	switch variant {
	case ElevatedCard:
		return Elevate(element.Background(Monochrome.SurfaceBright), 1)
	case FilledCard:
		return Elevate(element.Background(Monochrome.SurfaceContainer), 0)
	}
	return element
}

// Menu and Dialog style an existing modal element, preserving its anchoring,
// focus handling, scrolling and actions.
func Menu(element ui.Element) ui.Element {
	return Elevate(element.MinWidth(0).BorderWidth(0).Radius(ShapeMedium).Background(Monochrome.SurfaceLow), 2)
}

func Dialog(element ui.Element) ui.Element {
	return Elevate(element.BorderWidth(0).Radius(DialogRadius).Background(Monochrome.SurfaceLow), 3)
}

func FloatingAction(element ui.Element) ui.Element {
	return Elevate(element.BorderWidth(0).Radius(ShapeLarge).Background(Monochrome.PrimaryContainer).TextColor(Monochrome.OnPrimaryContainer), 3)
}
