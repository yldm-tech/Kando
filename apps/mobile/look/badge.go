package look

import (
	"strconv"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Count is a count on a capsule in danger's color, or warning's while Warn reports true, as the badge on a button or a section; it hides while the count is 0. Place it with Style, as at a button's top corner; the screen reader hears the count in the label of what it is on, so the badge itself is hidden from it.
type Count struct {
	Count  func() int
	Warn   func() bool
	Style  core.Style
	TestID string
}

// Render implements core.Node.
func (c Count) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: c.TestID, AccessibilityElementsHidden: func() bool { return true }, PointerEvents: core.PointerEventsNone,
		Style: core.Style{MinWidth: core.Pt(18), Height: core.Pt(18), Padding: core.Symmetric(0, 5), CornerRadius: 9, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}.Merge(c.Style),
		StyleFunc: func() core.Style {
			st := core.Style{Background: colors.Danger(), Color: colors.OnAccent()}
			if c.Warn != nil && c.Warn() {
				st.Background = colors.Warn()
			}
			if c.Count() == 0 {
				st.Display = core.DisplayNone
			}
			return st
		},
		Children: []core.Node{core.Text{ContentFunc: func() string { return strconv.Itoa(c.Count()) }, MaxLines: 1, Style: core.Style{FontSize: 11, LineHeight: 18, FontWeight: core.FontWeightBold}}}}
}
