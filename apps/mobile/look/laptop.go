package look

import (
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
)

// Laptop is the design's drawing of a computer, width wide: a dark lid around a blue screen over a thin base, as the sidebar's computer card, the Jobs page and pairing show a computer. It is a picture, hidden from assistive technology.
func Laptop(width core.Points) core.Node {
	lid := width * 26 / 40
	return core.View{AccessibilityElementsHidden: func() bool { return true }, PointerEvents: core.PointerEventsNone,
		Style: core.Style{Width: core.Pt(width), AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pct(86), Height: core.Pt(lid), CornerRadius: width / 12, Padding: core.All(width / 20),
				Gradients: []core.Gradient{core.LinearGradient(180, theme.LaptopTop, theme.LaptopBottom)}}, Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, CornerRadius: width / 24, Gradients: []core.Gradient{core.LinearGradient(160, theme.ScreenTop, theme.ScreenBottom)}}},
			}},
			core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pt(width * 3 / 40), CornerRadii: core.Corners{BottomLeft: width / 20, BottomRight: width / 20},
				Background: theme.LaptopTop.Faded(0.35)}},
		}}
}

// LaptopFront is the design's larger drawing of a computer, width wide, as the Library's card before pairing shows it: a lid of the full width, 52 high for 74 wide, around a blue screen, its base a light strip tucked under it as the design's shadow draws it. It is a picture, hidden from assistive technology.
func LaptopFront(width core.Points) core.Node {
	inset := width * 4 / 74
	return core.View{AccessibilityElementsHidden: func() bool { return true }, PointerEvents: core.PointerEventsNone,
		Style: core.Style{Width: core.Pt(width), Height: core.Pt(width * 52 / 74), CornerRadius: width * 7 / 74, Padding: core.Edges{Top: inset, Right: inset, Bottom: width * 7 / 74, Left: inset}, FlexShrink: 0,
			Gradients: []core.Gradient{core.LinearGradient(160, theme.LaptopTop, theme.LaptopBottom)},
			Shadows:   []core.BoxShadow{{Offset: core.Point{Y: width * 7 / 74}, Spread: -width * 4 / 74, Color: theme.LaptopBase}}},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, CornerRadius: width * 3 / 74, Gradients: []core.Gradient{core.LinearGradient(135, theme.ScreenTop, theme.ScreenBottom)}}},
		}}
}
