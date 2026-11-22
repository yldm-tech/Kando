package player

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// appSheet is a sheet over the player in the app's own appearance rather than the player's dark, as the design draws the word, the explanation and the fix: the page's grey under white cards, rounder at its top than ui.Sheet. Dark keeps the player's dark on the surface's color instead, as the playback settings. Label names it for screen readers. It is at most 86% of the screen tall, as the design's tallest; a Scroll among its children shrinks to fit it (give it FlexShrink 1 and MinHeight 0, with no MaxHeight of its own), so its foot is reached by scrolling rather than cut.
// TODO(nsgo): ui.Sheet now takes a Style and a StyleFunc for its surface (nsgo#1036), but no Label or LabelFunc for screen readers, no TestID and no CloseOnSwipe (ui/sheet.go:11-19), which base.Sheet has and this sheet uses, so it is base.Sheet with ui.Sheet's grabber and padding meanwhile; make it ui.Sheet with its fill, corners and height in StyleFunc once ui.Sheet passes those through
type appSheet struct {
	Dark     bool
	Open     func() bool
	OnClose  func()
	Label    func() string
	TestID   string
	Children []core.Node
}

// Render implements core.Node.
func (s appSheet) Render(cx *core.Scope) core.Node {
	if !s.Dark {
		appAppearance(cx)
	}
	colors := ui.ColorsOf(cx)
	env := cx.Env()
	grabber := core.View{Style: core.Style{Width: core.Pt(36), Height: core.Pt(5), CornerRadius: 3, AlignSelf: core.AlignCenter}, StyleFunc: ui.Filled(colors.Border)}
	return base.Sheet{TestID: s.TestID, Open: s.Open, LabelFunc: s.Label, CloseOnSwipe: true,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				s.OnClose()
			}
		},
		BackdropStyle: core.Style{Background: ui.Black(0.4)},
		PopupStyleFunc: func() core.Style {
			fill := colors.Background()
			if s.Dark {
				fill = colors.Surface()
			}
			return core.Style{Gap: 12, Padding: core.Edges{Top: 8, Right: 16, Bottom: env.SafeArea().Bottom + 16, Left: 16}, MaxHeight: core.Pct(86),
				CornerRadii: core.Corners{TopLeft: 26, TopRight: 26}, Background: fill, Color: colors.Text()}
		},
		Children: append([]core.Node{grabber}, s.Children...)}
}

// appAppearance shows cx's subtree in the app's appearance, light or dark as the app is, under the player's dark (theme.Dark).
func appAppearance(cx *core.Scope) {
	pick := func() core.Appearance {
		if cx.Env().Dark() {
			return core.AppearanceDark
		}
		return core.AppearanceLight
	}
	cx.SetSubtreeAppearance(core.Untracked(cx, pick))
	cx.Effect(func() { cx.SetSubtreeAppearance(pick()) })
}

// sheetCard is the style of a card in an appSheet: the surface, 14 round.
func sheetCard(colors ui.Colors) func() core.Style {
	return func() core.Style {
		return core.Style{Background: colors.Surface(), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous}
	}
}
