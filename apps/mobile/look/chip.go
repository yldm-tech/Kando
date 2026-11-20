package look

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Chip is a choice among others in a row that wraps: a capsule ringed in the accent while On, as a checkbox, or as a radio button where Radio is set. A Filter chip is the smaller one that narrows a list, as the Library's sources: 32 points tall on a card, filled with the ink while On.
type Chip struct {
	Text    func() string
	On      func() bool
	Radio   bool
	Filter  bool
	OnPress func()
	TestID  string
}

// Render implements core.Node.
func (c Chip) Render(cx *core.Scope) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	role := core.RoleCheckbox
	if c.Radio {
		role = core.RoleRadio
	}
	if c.Filter {
		return core.View{TestID: c.TestID, Role: role, LabelFunc: c.Text, Checked: c.On, OnPress: func(core.PressEvent) { c.OnPress() },
			Style: core.Style{MinHeight: core.Pt(32), Padding: core.Symmetric(0, 14), CornerRadius: 16, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style {
				if c.On() {
					return core.Style{Background: colors.Text(), Color: colors.Background()}
				}
				return core.Style{Background: colors.Surface(), Color: colors.Text()}
			},
			Children: []core.Node{core.Text{ContentFunc: c.Text, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 17, FontWeight: core.FontWeightSemibold}}}}
	}
	return core.View{TestID: c.TestID, Role: role, LabelFunc: c.Text, Checked: c.On, OnPress: func(core.PressEvent) { c.OnPress() },
		Style: core.Style{MinHeight: core.Pt(44), Padding: core.Symmetric(0, 18), CornerRadius: 22, BorderCurve: core.BorderCurveContinuous, BorderWidth: 2,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style {
			if c.On() {
				return core.Style{Background: tints.Accent(), BorderColor: colors.Accent(), Color: colors.Accent()}
			}
			return core.Style{Background: colors.Surface(), BorderColor: ui.Alpha(colors.Text(), 0), Color: colors.Text()}
		},
		Children: []core.Node{core.Text{ContentFunc: c.Text, MaxLines: 1, Style: Body.Merge(core.Style{FontWeight: core.FontWeightMedium})}}}
}

// Chips lays chips out in rows that wrap.
func Chips(children ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 10}, Children: children}
}
