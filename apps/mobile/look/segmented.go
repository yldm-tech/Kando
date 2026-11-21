package look

import (
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Segment is one choice of a Segmented: its value, its text and its test ID.
type Segment struct {
	Value  string
	Text   func() string
	TestID string
}

// Segmented is a choice of one among a few, as the Add sheet's Save: the design's segmented control, 36 points tall on the fill, 10 round, with the chosen segment on a white thumb 8 round and in bold. Each segment is a radio button of the group Label names.
// TODO(nsgo#145): ui.Tabs' Label and its options' Label are strings only (ui/tabs.go:42, ui/option.go:8), so a segment's and the group's spoken names would keep the language they were built in, and the segments are drawn here with LabelFunc; use ui.Tabs with SegmentTrack, SegmentTab and SegmentThumb once they take a LabelFunc https://github.com/nsgo-dev/nsgo/issues/145
type Segmented struct {
	Segments []Segment
	Value    func() string
	OnChange func(string)
	Label    func() string
}

// Render implements core.Node.
func (s Segmented) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	var children []core.Node
	for _, seg := range s.Segments {
		on := func() bool { return s.Value() == seg.Value }
		children = append(children, core.View{TestID: seg.TestID, Role: core.RoleRadio, LabelFunc: seg.Text, Checked: on, OnPress: func(core.PressEvent) { s.OnChange(seg.Value) },
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 6), CornerRadius: 8, BorderCurve: core.BorderCurveContinuous},
			StyleFunc: func() core.Style {
				if on() {
					return core.Style{Background: colors.Raised(), Color: colors.Text(), Shadows: []core.BoxShadow{{Offset: core.Point{Y: 1}, Blur: 3, Color: theme.SegmentShadow}}}
				}
				return core.Style{Color: colors.Text(), Active: &core.Style{Opacity: core.Alpha(0.6)}}
			},
			Children: []core.Node{core.Text{ContentFunc: seg.Text, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18},
				StyleFunc: func() core.Style {
					if on() {
						return core.Style{FontWeight: core.FontWeightBold}
					}
					return core.Style{FontWeight: core.FontWeightMedium}
				}}}})
	}
	return core.View{Role: core.RoleRadioGroup, LabelFunc: s.Label, Children: children,
		Style:     core.Style{Direction: core.Row, Height: core.Pt(36), Padding: core.All(2), Gap: 2, CornerRadius: 10, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: ui.Filled(colors.Inset)}
}

// SegmentTrack, SegmentTab and SegmentThumb give ui.Tabs' segmented look the design's: a 10-round track under an 8-round thumb lifted by a 0 1 3 shadow of 0.12 black, as the Style, TabStyle and IndicatorStyle of the control.
var (
	SegmentTrack = core.Style{CornerRadius: 10, BorderCurve: core.BorderCurveContinuous}
	SegmentTab   = core.Style{CornerRadius: 8, BorderCurve: core.BorderCurveContinuous}
	SegmentThumb = core.Style{CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, Shadows: []core.BoxShadow{{Offset: core.Point{Y: 1}, Blur: 3, Color: theme.SegmentShadow}}}
)

// RadioMark is the design's radio circle at a choice's end, 22 points: a ring of the accent around its dot while on, a light grey ring while off. It repeats the row's checked state, so assistive technology does not see it.
func RadioMark(cx *core.Scope, on func() bool) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{AccessibilityElementsHidden: func() bool { return true }, PointerEvents: core.PointerEventsNone,
		Style: core.Style{Width: core.Pt(22), Height: core.Pt(22), CornerRadius: 11, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{BorderWidth: 2, BorderColor: colors.Accent()}
			}
			return core.Style{BorderWidth: 1.5, BorderColor: theme.Chevron}
		},
		Children: []core.Node{core.Show{When: on, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(12), Height: core.Pt(12), CornerRadius: 6}, StyleFunc: ui.Filled(colors.Accent)}
		}}}}
}
