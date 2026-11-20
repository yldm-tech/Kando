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
