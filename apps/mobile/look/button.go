package look

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Kind is a Button's look.
type Kind uint8

const (
	// Primary is filled with the accent, for a page's main action.
	Primary Kind = iota
	// Tinted is the accent on its tint, for an action beside the main one or one a page offers quietly.
	Tinted
	// Danger is danger's color on its tint, for an action that takes something away.
	Danger
	// Plain is the accent's text alone, for a way out under the main action.
	Plain
)

// Button is a phone's full-width button, ui.Button 50 points tall and 14 round (44 Plain) in the design's 17pt semibold, with an optional leading icon. A Tall one is the introduction's call to action at a screen's foot: 56 points tall, 16 round, its text bold, and a Primary one lifted by a shadow in the accent. While Busy it shows a spinner in the button's text color before BusyLabel, or before its Text without one, and takes no presses; assistive technology reads the label shown.
type Button struct {
	Text      func() string
	Icon      func() *core.SVG
	Kind      Kind
	Busy      func() bool
	BusyLabel func() string
	Disabled  func() bool
	// Flex shares a row with the buttons beside it in proportion.
	Flex    float32
	Tall    bool
	OnPress func()
	TestID  string
}

// Render implements core.Node.
func (b Button) Render(cx *core.Scope) core.Node {
	variant := map[Kind]ui.ButtonVariant{Primary: ui.ButtonPrimary, Tinted: ui.ButtonTinted, Danger: ui.ButtonDanger, Plain: ui.ButtonGhost}[b.Kind]
	// A button's TextStyle and Style are read once, so what they take of the accent is the accent as it is built: Kando's is the same in both appearances (theme.Kando).
	accent := ui.ColorsOf(cx).Accent()
	shape := core.Style{Height: core.Pt(50), CornerRadius: 14}
	text := core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightSemibold}
	var tint func() core.Color
	if b.Kind == Plain {
		// A ghost's text is the ink; Plain's is the accent.
		shape.Height, text.Color, tint = core.Pt(44), accent, ui.Fixed(accent)
	}
	if b.Tall {
		shape.Height, shape.CornerRadius, text.FontWeight = core.Pt(56), 16, core.FontWeightBold
		if b.Kind == Primary {
			shape.Shadows = []core.BoxShadow{{Offset: core.Point{Y: 10}, Blur: 24, Color: ui.Alpha(accent, 0.3)}}
		}
	}
	if b.Flex > 0 {
		shape.FlexBasis, shape.MinWidth = core.Pt(0), core.Pt(0)
	}
	busyLabel := b.BusyLabel
	if busyLabel == nil {
		// Without a label of its own a busy button keeps its text beside the spinner, which takes the icon's place.
		busyLabel = b.Text
	}
	return ui.Button{TestID: b.TestID, TextFunc: b.Text, TextStyle: text, Variant: variant, Icon: b.Icon, IconSize: 20, Tint: tint,
		Busy: b.Busy, BusyLabelFunc: busyLabel, Disabled: b.Disabled, Flex: b.Flex, Size: ui.SizeLarge, Style: shape, OnPress: b.OnPress}
}

// Pill is a small capsule button, for a way out at a screen's top, as Skip.
func Pill(cx *core.Scope, testID string, text func() string, press func()) core.Node {
	colors := ui.ColorsOf(cx)
	return base.Button{TestID: testID, LabelFunc: text, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{MinHeight: core.Pt(32), Padding: core.Symmetric(0, 14), CornerRadius: 16, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Inset(), Color: colors.Text()} },
		Children:  []core.Node{core.Text{ContentFunc: text, MaxLines: 1, Style: Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold})}}}
}

// CircleButton is a round button of a page's header, 34 points in a 44-point target: the accent's icon on a card ringed by a hairline, or with Primary a white 14-point icon (Plus) on the accent, lifted by its shadow, for the page's main action. Badge, when set, sits at its top corner, as Count.
type CircleButton struct {
	Icon    func() *core.SVG
	Label   func() string
	Primary bool
	Badge   core.Node
	OnPress func()
	TestID  string
}

// Render implements core.Node.
func (b CircleButton) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	// The primary circle's glyph is the design's 14-point one (Plus); the card's is 17.
	size := core.Points(17)
	if b.Primary {
		size = 14
	}
	face := core.View{Style: core.Style{Width: core.Pt(34), Height: core.Pt(34), CornerRadius: 17, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: func() core.Style {
			if b.Primary {
				return core.Style{Background: colors.Accent(), Color: colors.OnAccent(), Shadows: []core.BoxShadow{{Offset: core.Point{Y: 4}, Blur: 12, Color: ui.Alpha(colors.Accent(), 0.35)}}}
			}
			return core.Style{Background: colors.Surface(), Color: colors.Accent(), Shadows: []core.BoxShadow{core.Ring(0.5, ui.Alpha(colors.Text(), 0.08))}}
		},
		Children: []core.Node{ui.Icon(b.Icon, size, ui.Inherited)}}
	children := []core.Node{face}
	if b.Badge != nil {
		children = append(children, b.Badge)
	}
	return base.Button{TestID: b.TestID, LabelFunc: b.Label, OnPress: func(core.PressEvent) { b.OnPress() },
		Style:     core.Style{Width: core.Pt(44), Height: core.Pt(44), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return core.Style{FocusVisible: ui.FocusRing(cx, -2)} },
		Children:  children}
}
