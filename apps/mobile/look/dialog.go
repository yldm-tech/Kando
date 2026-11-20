package look

import (
	"time"

	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Dialog is a form over the app, as the Add sheet: a sheet from the bottom on a phone, and on a tablet a card in the middle of the screen, as the design's Pad artboards draw it, clear of the software keyboard. Footer is its actions, or with Nav its bar at the top holds them instead and Footer is not drawn. Header, when set, is drawn in the title's place, as a film's poster with its title; Title still names the sheet for assistive technology.
// TODO(nsgo#742): ui.Dialog is a bottom sheet wherever the screen is touched (ui/dialog.go:73), a tablet's too, with no way to ask for its centred dialog; the tablet's is drawn here on base.Dialog until ui.Dialog takes a presentation. https://github.com/nsgo-dev/nsgo/issues/742
type Dialog struct {
	Open      func() bool
	OnClose   func()
	OnConfirm func()
	Title     func() string
	Children  []core.Node
	Footer    []core.Node
	Header    core.Node
	// Nav, when set, heads the dialog with the design's sheet bar in place of its title: Cancel, the title in the middle and the confirming action.
	Nav    *SheetNav
	TestID string
}

// SheetNav is the bar at the top of a sheet, as the design's Add sheet: Cancel at its start, the sheet's title in the middle and the action that confirms it at its end, 17 points, the action bold and grey while Disabled. While Busy the action shows a spinner and takes no presses.
// TODO(nsgo): ui.Dialog has no slots in its title row for a leading and a trailing action (ui/dialog.go:61-70), so the bar is drawn as the sheet's first child, and on iOS it is not the system's sheet navigation bar.
type SheetNav struct {
	Cancel    func() string
	Confirm   func() string
	Disabled  func() bool
	Busy      func() bool
	OnCancel  func()
	OnConfirm func()
	CancelID  string
	ConfirmID string
}

// bar is the nav's row titled title.
func (n SheetNav) bar(cx *core.Scope, title func() string) core.Node {
	colors := ui.ColorsOf(cx)
	off := func() bool { return (n.Disabled != nil && n.Disabled()) || (n.Busy != nil && n.Busy()) }
	text := core.Style{FontSize: 17, LineHeight: 22}
	button := core.Style{MinHeight: core.Pt(44), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Direction: core.Row, Active: &core.Style{Opacity: core.Alpha(0.6)}}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, MinHeight: core.Pt(44)}, Children: []core.Node{
		// The title is centered across the whole bar, whatever the buttons' widths, under them.
		core.Text{ContentFunc: title, Role: core.RoleHeader, MaxLines: 1, PointerEvents: core.PointerEventsNone,
			Style:     text.Merge(core.Style{FontWeight: core.FontWeightBold, TextAlign: core.TextAlignCenter, Position: core.Absolute, Left: core.Pt(80), Right: core.Pt(80)}),
			StyleFunc: ui.Colored(colors.Text)},
		base.Button{TestID: n.CancelID, LabelFunc: n.Cancel, OnPress: func(core.PressEvent) { n.OnCancel() }, Style: button, StyleFunc: ui.Colored(colors.Accent),
			Children: []core.Node{core.Text{ContentFunc: n.Cancel, MaxLines: 1, Style: text}}},
		base.Button{TestID: n.ConfirmID, LabelFunc: n.Confirm, OnPress: func(core.PressEvent) { n.OnConfirm() }, Disabled: off, Style: button,
			StyleFunc: func() core.Style {
				if n.Disabled != nil && n.Disabled() {
					return core.Style{Color: theme.NavDisabled}
				}
				return core.Style{Color: colors.Accent()}
			},
			Children: []core.Node{core.Show{When: func() bool { return n.Busy != nil && n.Busy() },
				Then: func(*core.Scope) core.Node { return ui.Spinner{Size: 20, Tint: colors.Accent} },
				Else: func(cx *core.Scope) core.Node {
					return core.Text{ContentFunc: n.Confirm, MaxLines: 1, Style: text.Merge(core.Style{FontWeight: core.FontWeightBold})}
				}}}},
	}}
}

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	change := func(open bool, _ base.Reason) {
		if !open {
			d.OnClose()
		}
	}
	colors := ui.ColorsOf(cx)
	// The title is the design's 17-point heavy one, ui.Dialog's own being TypeTitle (ui/dialog.go:64): the sheet draws it as its first child and is named by it through LabelFunc.
	var title core.Node = core.Text{ContentFunc: d.Title, Role: core.RoleHeader, Style: Title.Merge(core.Style{FontWeight: Heavy}), StyleFunc: ui.Colored(colors.Text)}
	if d.Header != nil {
		title = d.Header
	}
	footer := d.Footer
	if d.Nav != nil {
		title, footer = d.Nav.bar(cx, d.Title), nil
	}
	if !Tablet(cx) {
		// A sheet is the page's grey, so the cards in it stand out as on a page, rounded more than a card at its top.
		// TODO(nsgo#933): the touch sheet's grabber, padding, height, shadow and motion are fixed (ui/style.go:114-155, sheetPanel and sheetContent), so the sheet keeps NSGo's 36x5 grabber and 16pt padding rather than the design's 38x5 grabber and 6/16/34 padding. https://github.com/nsgo-dev/nsgo/issues/933
		return ui.Dialog{TestID: d.TestID, Open: d.Open, LabelFunc: d.Title, OnOpenChange: change, OnConfirm: d.OnConfirm, Children: append([]core.Node{title}, d.Children...), Footer: footer,
			StyleFunc: func() core.Style {
				st := core.Style{Background: colors.Background()}
				if ui.Touch(cx) {
					st.CornerRadii = core.Corners{TopLeft: 26, TopRight: 26}
				}
				return st
			}}
	}
	body := append([]core.Node{title}, d.Children...)
	if footer != nil {
		body = append(body, core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifyEnd, Gap: 8, Margin: core.Edges{Top: 4}}, Children: footer})
	}
	// The design's sheet curve, which the card pops in on.
	curve := core.Curve{X1: 0.32, Y1: 0.72, X2: 0, Y2: 1}
	fade := core.Transition{Duration: 300 * time.Millisecond, Easing: curve, Properties: core.TransitionOpacity}
	pop := core.Transition{Duration: 300 * time.Millisecond, Easing: curve}
	grabber := core.View{Style: core.Style{Width: core.Pt(38), Height: core.Pt(5), CornerRadius: 3, AlignSelf: core.AlignCenter, Margin: core.Edges{Top: 8}, Background: theme.Grabber}}
	return base.Dialog{TestID: d.TestID, Open: d.Open, OnOpenChange: change, OnConfirm: d.OnConfirm, Animation: core.ModalNone, Role: core.RoleDialog, LabelFunc: d.Title,
		BackdropStyle: core.Style{Background: ui.Black(0.4), Transition: fade}, BackdropStartingStyle: core.Style{Opacity: core.Alpha(0)}, BackdropEndingStyle: core.Style{Opacity: core.Alpha(0)},
		StartingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.94}}, EndingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.98}},
		LayerStyle: core.Style{Padding: core.All(24), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvKeyboard}},
		PopupStyle: core.Style{Width: core.Pt(560), MaxWidth: core.Pct(100), MaxHeight: core.Pct(88), Overflow: core.Hidden, CornerRadius: 24, BorderCurve: core.BorderCurveContinuous, Transition: pop,
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: -10}, Blur: 30, Color: theme.SheetShadow}}},
		PopupStyleFunc: func() core.Style {
			return core.Style{Background: colors.Background(), Color: colors.Text()}
		},
		Children: []core.Node{grabber, core.Scroll{Style: core.Style{FlexShrink: 1}, Children: []core.Node{core.View{Style: core.Style{Gap: 16, Padding: core.Edges{Top: 6, Right: 16, Bottom: 34, Left: 16}}, Children: body}}}}}
}
