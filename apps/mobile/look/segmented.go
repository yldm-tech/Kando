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

// Segmented is a choice of one among a few, as the Add sheet's Save: the design's segmented control, ui.Tabs 36 points tall, 10 round, with the chosen segment on a white thumb 8 round and in bold. Label names it for assistive technology, and each segment's Text names the segment.
type Segmented struct {
	Segments []Segment
	Value    func() string
	OnChange func(string)
	Label    func() string
}

// Render implements core.Node.
func (s Segmented) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	var options []ui.Option[string]
	for _, seg := range s.Segments {
		options = append(options, ui.Option[string]{Value: seg.Value, LabelFunc: seg.Text, TestID: seg.TestID,
			// The text may shrink below its own width, so a segment's text longer than the segment ends in an ellipsis inside it: the segment centers its content in a row, which a text that keeps its width overflows on both sides.
			Content: core.Text{ContentFunc: seg.Text, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18, FlexShrink: 1, MinWidth: core.Pt(0)},
				StyleFunc: func() core.Style {
					if s.Value() == seg.Value {
						return core.Style{FontWeight: core.FontWeightBold, Color: colors.Text()}
					}
					return core.Style{FontWeight: core.FontWeightMedium, Color: colors.Text()}
				}}})
	}
	return ui.Tabs[string]{Variant: ui.TabsSegmented, Equal: true, Size: ui.SizeLarge, LabelFunc: s.Label, Options: options, Value: s.Value, OnChange: s.OnChange,
		Style: SegmentTrack, TabStyle: SegmentTab.Merge(core.Style{Padding: core.Symmetric(0, 6)}), IndicatorStyle: SegmentThumb}
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
