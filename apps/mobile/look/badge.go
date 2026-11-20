package look

import (
	"strconv"

	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Count is a count on a capsule in the badges' red, or their orange while Warn reports true, as the badge on a button or a section; it hides while the count is 0. Size is its height, 18 when 0 (a tab bar's 18 or 20, a sidebar row's 22), and while OnAccent reports true it sits on the accent's fill, as on a selected sidebar row, in white over a quarter of white. Place it with Style, as at a button's top corner; the screen reader hears the count in the label of what it is on, so the badge itself is hidden from it.
type Count struct {
	Count    func() int
	Warn     func() bool
	Size     core.Points
	OnAccent func() bool
	Style    core.Style
	TestID   string
}

// Render implements core.Node.
func (c Count) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	size := c.Size
	if size == 0 {
		size = 18
	}
	// The text grows with the capsule: 11pt up to 20, 12pt on a sidebar's 22.
	font, pad := core.Points(11), core.Points(5)
	if size >= 22 {
		font, pad = 12, 7
	}
	return core.View{TestID: c.TestID, AccessibilityElementsHidden: func() bool { return true }, PointerEvents: core.PointerEventsNone,
		Style: core.Style{MinWidth: core.Pt(size), Height: core.Pt(size), Padding: core.Symmetric(0, pad), CornerRadius: size / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}.Merge(c.Style),
		StyleFunc: func() core.Style {
			st := core.Style{Background: theme.BadgeRed, Color: colors.OnAccent()}
			if c.Warn != nil && c.Warn() {
				st.Background = theme.BadgeOrange
			}
			if c.OnAccent != nil && c.OnAccent() {
				st.Background = theme.BadgeOnAccent
			}
			if c.Count() == 0 {
				st.Display = core.DisplayNone
			}
			return st
		},
		Children: []core.Node{core.Text{ContentFunc: func() string { return strconv.Itoa(c.Count()) }, MaxLines: 1, Style: core.Style{FontSize: font, LineHeight: size, FontWeight: core.FontWeightBold}}}}
}
