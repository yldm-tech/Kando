package look

import (
	"time"

	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Dialog is a form over the app, as the Add sheet: a sheet from the bottom on a phone, and on a tablet a card in the middle of the screen, as the design's Pad artboards draw it, clear of the software keyboard. Footer is its actions, or with Nav its bar at the top holds them instead and Footer is not drawn. Header, when set, is drawn in the title's place, as a film's poster with its title; Title still names the sheet for assistive technology.
// ui.Dialog is a bottom sheet wherever the screen is touched (ui/dialog.go:73), a tablet's too, with no way to ask for its centred dialog, so the tablet's is drawn here on base.Dialog. NSGo declined a presentation option for now (nsgo#742, closed).
type Dialog struct {
	Open      func() bool
	OnClose   func()
	OnConfirm func()
	Title     func() string
	// TitleHidden leaves the title out of the sheet; assistive technology still names the sheet by it.
	TitleHidden bool
	Children    []core.Node
	Footer      []core.Node
	Header      core.Node
	// Nav, when set, heads the dialog with the design's sheet bar in place of its title: Cancel, the title in the middle and the confirming action.
	Nav    *SheetNav
	TestID string
}

// SheetNav is the bar at the top of a sheet, as the design's Add sheet: Cancel at its start, the sheet's title in the middle and the action that confirms it at its end, 17 points, the action bold and grey while Disabled. While Busy the action shows a spinner and takes no presses. Both buttons show the focus ring look.Button does while the keyboard is on them. On a phone they are ui.Dialog's Leading and Trailing, beside its title; on a tablet's centred dialog the bar is drawn as its first child.
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

// navText is the bar's type.
var navText = core.Style{FontSize: 17, LineHeight: 22}

// buttons are the nav's Cancel and its confirming action.
func (n SheetNav) buttons(cx *core.Scope) (cancel, confirm core.Node) {
	colors := ui.ColorsOf(cx)
	off := func() bool { return (n.Disabled != nil && n.Disabled()) || (n.Busy != nil && n.Busy()) }
	button := core.Style{MinHeight: core.Pt(44), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Direction: core.Row, Active: &core.Style{Opacity: core.Alpha(0.6)}}
	cancel = base.Button{TestID: n.CancelID, LabelFunc: n.Cancel, OnPress: func(core.PressEvent) { n.OnCancel() }, Style: button,
		StyleFunc: func() core.Style { return core.Style{Color: colors.Accent(), FocusVisible: ui.FocusRing(cx, -2)} },
		Children:  []core.Node{core.Text{ContentFunc: n.Cancel, MaxLines: 1, Style: navText}}}
	confirm = base.Button{TestID: n.ConfirmID, LabelFunc: n.Confirm, OnPress: func(core.PressEvent) { n.OnConfirm() }, Disabled: off, Style: button,
		StyleFunc: func() core.Style {
			st := core.Style{Color: colors.Accent(), FocusVisible: ui.FocusRing(cx, -2)}
			if n.Disabled != nil && n.Disabled() {
				st.Color = theme.NavDisabled
			}
			return st
		},
		Children: []core.Node{core.Show{When: func() bool { return n.Busy != nil && n.Busy() },
			Then: func(*core.Scope) core.Node { return ui.Spinner{Size: 20, Tint: colors.Accent} },
			Else: func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: n.Confirm, MaxLines: 1, Style: navText.Merge(core.Style{FontWeight: core.FontWeightBold})}
			}}}}
	return cancel, confirm
}

// bar is the nav's row titled title, as a tablet's centred dialog draws it.
func (n SheetNav) bar(cx *core.Scope, title func() string) core.Node {
	colors := ui.ColorsOf(cx)
	cancel, confirm := n.buttons(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, MinHeight: core.Pt(44)}, Children: []core.Node{
		// The title is centered across the whole bar, whatever the buttons' widths, under them.
		core.Text{ContentFunc: title, Role: core.RoleHeader, MaxLines: 1, PointerEvents: core.PointerEventsNone,
			Style:     navText.Merge(core.Style{FontWeight: core.FontWeightBold, TextAlign: core.TextAlignCenter, Position: core.Absolute, Left: core.Pt(80), Right: core.Pt(80)}),
			StyleFunc: ui.Colored(colors.Text)},
		cancel, confirm,
	}}
}

// grabber is the design's handle at a sheet's top, 38 by 5, 8 points down.
func grabber(st core.Style) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(38), Height: core.Pt(5), CornerRadius: 3, AlignSelf: core.AlignCenter, Background: theme.Grabber}.Merge(st)}
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
	var head []core.Node
	if !d.TitleHidden {
		head = []core.Node{core.Text{ContentFunc: d.Title, Role: core.RoleHeader, Style: Title.Merge(core.Style{FontWeight: Heavy}), StyleFunc: ui.Colored(colors.Text)}}
	}
	if d.Header != nil {
		head = []core.Node{d.Header}
	}
	footer := d.Footer
	if !Tablet(cx) {
		dialog := ui.Dialog{TestID: d.TestID, Open: d.Open, LabelFunc: d.Title, OnOpenChange: change, OnConfirm: d.OnConfirm, Footer: footer,
			StyleFunc: func() core.Style {
				st := core.Style{Background: colors.Background()}
				if ui.Touch(cx) {
					// A sheet is the page's grey, so the cards in it stand out as on a page, rounded more than a card at its top; under the design's grabber its content is padded 6, 16 and 34, as the tablet's card, its foot clear of the safe area.
					st.CornerRadii = core.Corners{TopLeft: 26, TopRight: 26}
					st.Padding, st.Gap = core.Edges{Top: 8 + 5 + 6, Right: 16, Bottom: max(34, cx.Env().SafeArea().Bottom), Left: 16}, 16
				}
				return st
			}}
		if d.Nav != nil {
			cancel, confirm := d.Nav.buttons(cx)
			dialog.TitleFunc, dialog.TitleStyle, dialog.Leading, dialog.Trailing, dialog.Footer = d.Title, navText.Merge(core.Style{FontWeight: core.FontWeightBold}), []core.Node{cancel}, []core.Node{confirm}, nil
			head = nil
		}
		children := append(head, d.Children...)
		if ui.Touch(cx) {
			dialog.Bare = true
			children = append([]core.Node{core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Left: core.Pt(0), Right: core.Pt(0), AlignItems: core.AlignCenter},
				Children: []core.Node{grabber(core.Style{})}}}, children...)
		}
		dialog.Children = children
		return dialog
	}
	if d.Nav != nil {
		head, footer = []core.Node{d.Nav.bar(cx, d.Title)}, nil
	}
	body := append(head, d.Children...)
	if footer != nil {
		body = append(body, core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifyEnd, Gap: 8, Margin: core.Edges{Top: 4}}, Children: footer})
	}
	// The design's sheet curve, which the card pops in on.
	curve := core.Curve{X1: 0.32, Y1: 0.72, X2: 0, Y2: 1}
	fade := core.Transition{Duration: 300 * time.Millisecond, Easing: curve, Properties: core.TransitionOpacity}
	pop := core.Transition{Duration: 300 * time.Millisecond, Easing: curve}
	return base.Dialog{TestID: d.TestID, Open: d.Open, OnOpenChange: change, OnConfirm: d.OnConfirm, Animation: core.ModalNone, Role: core.RoleDialog, LabelFunc: d.Title,
		BackdropStyle: core.Style{Background: ui.Black(0.4), Transition: fade}, BackdropStartingStyle: core.Style{Opacity: core.Alpha(0)}, BackdropEndingStyle: core.Style{Opacity: core.Alpha(0)},
		StartingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.94}}, EndingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.98}},
		LayerStyle: core.Style{Padding: core.All(24), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvKeyboard}},
		PopupStyle: core.Style{Width: core.Pt(560), MaxWidth: core.Pct(100), MaxHeight: core.Pct(88), Overflow: core.Hidden, CornerRadius: 24, BorderCurve: core.BorderCurveContinuous, Transition: pop,
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: -10}, Blur: 30, Color: theme.SheetShadow}}},
		PopupStyleFunc: func() core.Style {
			return core.Style{Background: colors.Background(), Color: colors.Text()}
		},
		Children: []core.Node{grabber(core.Style{Margin: core.Edges{Top: 8}}), core.Scroll{Style: core.Style{FlexShrink: 1}, Children: []core.Node{core.View{Style: core.Style{Gap: 16, Padding: core.Edges{Top: 6, Right: 16, Bottom: 34, Left: 16}}, Children: body}}}}}
}
