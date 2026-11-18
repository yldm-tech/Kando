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

// Button is a phone's full-width capsule button, 50 points tall (44 Plain), with an optional leading icon. While Busy it shows a spinner in the icon's place, in the button's text color, and takes no presses, and BusyLabel, when set, is what assistive technology reads meanwhile.
// TODO(nsgo#482): ui.Button is 32pt tall at most (size.go:10-15,31) and takes no Style (button.go:28-50), so a phone's large or full-width button is base.Button with the app's own style; use ui.Button once it has a size or style for that https://github.com/nsgo-dev/nsgo/issues/482
type Button struct {
	Text      func() string
	Icon      func() *core.SVG
	Kind      Kind
	Busy      func() bool
	BusyLabel func() string
	Disabled  func() bool
	// Flex shares a row with the buttons beside it in proportion.
	Flex    float32
	OnPress func()
	TestID  string
}

// Render implements core.Node.
func (b Button) Render(cx *core.Scope) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	busy := func() bool { return b.Busy != nil && b.Busy() }
	shape := core.Style{Height: core.Pt(50), CornerRadius: 25, BorderCurve: core.BorderCurveContinuous, Direction: core.Row, Gap: 8,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 20),
		Active: &core.Style{Opacity: core.Alpha(0.7)}}
	if b.Kind == Plain {
		shape.Height = core.Pt(44)
	}
	if b.Flex > 0 {
		shape.FlexGrow, shape.FlexShrink, shape.FlexBasis, shape.MinWidth = b.Flex, 1, core.Pt(0), core.Pt(0)
	}
	kindStyle := func() core.Style {
		switch b.Kind {
		case Tinted:
			return core.Style{Background: tints.Accent(), Color: colors.Accent()}
		case Danger:
			return core.Style{Background: tints.Danger(), Color: colors.Danger()}
		case Plain:
			return core.Style{Color: colors.Accent()}
		}
		return core.Style{Background: colors.Accent(), Color: colors.OnAccent()}
	}
	look := func() core.Style {
		st := kindStyle()
		// A busy button takes no presses but stays whole, as the platforms' own do: only one that cannot be pressed fades.
		if !busy() {
			st.Disabled = &core.Style{Opacity: core.Alpha(0.4)}
		}
		return st
	}
	// ink is the button's text color, which its spinner takes.
	ink := func() core.Color {
		switch b.Kind {
		case Primary:
			return colors.OnAccent()
		case Danger:
			return colors.Danger()
		}
		return colors.Accent()
	}
	var lead core.Node
	if b.Icon != nil {
		lead = ui.Icon(b.Icon, 20, ui.Inherited)
	}
	label := b.Text
	if b.BusyLabel != nil {
		label = func() string {
			if busy() {
				return b.BusyLabel()
			}
			return b.Text()
		}
	}
	return base.Button{TestID: b.TestID, LabelFunc: label, Style: shape, StyleFunc: look,
		OnPress:  func(core.PressEvent) { b.OnPress() },
		Disabled: func() bool { return busy() || (b.Disabled != nil && b.Disabled()) },
		Children: []core.Node{
			core.Show{When: busy, Then: func(*core.Scope) core.Node { return ui.Spinner{Size: 20, Tint: ink} }, Else: func(*core.Scope) core.Node { return lead }},
			core.Text{ContentFunc: b.Text, MaxLines: 1, Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightSemibold, FlexShrink: 1}},
		}}
}

// Pill is a small capsule button, for a way out at a screen's top, as Skip.
func Pill(cx *core.Scope, testID string, text func() string, press func()) core.Node {
	colors := ui.ColorsOf(cx)
	return base.Button{TestID: testID, LabelFunc: text, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{MinHeight: core.Pt(32), Padding: core.Symmetric(0, 14), CornerRadius: 16, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.07), Color: colors.Text()} },
		Children:  []core.Node{core.Text{ContentFunc: text, MaxLines: 1, Style: Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold})}}}
}
