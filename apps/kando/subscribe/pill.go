package subscribe

import (
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Look is a pill's fill and ink, as the design draws the page's and the dialog's buttons.
type Look uint8

const (
	// LookPrimary is the accent with white text, for the main action.
	LookPrimary Look = iota
	// LookSecondary is a quiet grey with the text's color.
	LookSecondary
	// LookDanger is a pale red with red text, for 取消订阅.
	LookDanger
	// LookTint is a pale accent with the accent's ink in bold, for a row's 加入.
	LookTint
	// LookQuiet is the secondary text alone, for a row's 跳过.
	LookQuiet
)

// pill is a fully rounded button in the design's sizes: Large, 34 points high in 14-point text, leads a card or a dialog; the rest are 28 points high in 13-point text. It is base.Button with Kando's own look, as ui.Button takes no text color (LookDanger, LookTint, LookQuiet) and draws its corners at the control radius. Its label follows Text without the button being built again.
type pill struct {
	Text func() string
	// Label is what assistive technology reads, Text when empty.
	Label    string
	Look     Look
	Large    bool
	Disabled func() bool
	OnPress  func()
	TestID   string
}

// Render implements ns.Node.
func (b pill) Render(cx *ns.Scope) ns.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	height, font, pad := ns.Points(28), ns.Points(13), ns.Points(14)
	if b.Large {
		height, font, pad = 34, 14, 18
	}
	weight := ui.Regular
	if b.Look == LookPrimary || b.Look == LookTint {
		weight = ui.Semibold
	}
	label := b.Text
	if b.Label != "" {
		label = func() string { return b.Label }
	}
	var press func(ns.PressEvent)
	if b.OnPress != nil {
		press = func(ns.PressEvent) { b.OnPress() }
	}
	// fill is the face at rest and under the pointer, and ink its text.
	fill := func() (rest, hover, ink ns.Color) {
		switch b.Look {
		case LookPrimary:
			p := cx.Palette()
			return p.Accent, p.AccentHover, colors.OnAccent()
		case LookDanger:
			red := k.DangerInk()
			return red.Faded(0.10), red.Faded(0.16), red
		case LookTint:
			return colors.Accent().Faded(0.12), colors.Accent().Faded(0.18), k.AccentInk()
		case LookQuiet:
			return ns.Transparent, k.Shade(0.05), colors.Muted()
		}
		return k.Shade(0.06), k.Shade(0.10), colors.Text()
	}
	return base.Button{TestID: b.TestID, LabelFunc: label, OnPress: press, Disabled: b.Disabled,
		Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOff},
		Style: ns.Style{Height: ns.Pt(height), Padding: ns.Symmetric(0, pad), CornerRadius: height / 2, FlexShrink: 0, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter,
			Cursor: ns.CursorDefault, FocusVisible: ui.FocusRing(cx, 2), Disabled: &ns.Style{Opacity: ns.Alpha(0.45)}},
		StyleFunc: func() ns.Style {
			rest, hover, _ := fill()
			return ns.Style{Background: rest, Hover: &ns.Style{Background: hover}}
		},
		Children: []ns.Node{ns.Text{ContentFunc: b.Text, MaxLines: 1, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: font, LineHeight: font + 5, FontWeight: weight},
			StyleFunc: func() ns.Style { _, _, ink := fill(); return ns.Style{Color: ink} }}}}
}

// card is a white card of the page that stands off it, as the design draws them: a hairline round it and a soft shadow under it, corners of 14.
func card(cx *ns.Scope, style ns.Style, children ...ns.Node) ns.Node {
	k := theme.Of(cx)
	style.CornerRadius, style.AlignItems, style.FlexShrink, style.BorderWidth = 14, ns.AlignStretch, 0, 1
	return ns.View{Style: style, StyleFunc: func() ns.Style {
		return ns.Style{Background: k.Card(), BorderColor: k.Shade(0.10), Shadows: []ns.BoxShadow{theme.ShadowRaised}}
	}, Children: children}
}
