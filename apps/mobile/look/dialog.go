package look

import (
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Dialog is a form over the app, as the Add sheet: a sheet from the bottom on a phone, and on a tablet a card in the middle of the screen, as the design's Pad artboards draw it, clear of the software keyboard. Footer is its actions.
// TODO(nsgo#742): ui.Dialog is a bottom sheet wherever the screen is touched (ui/dialog.go:73), a tablet's too, with no way to ask for its centred dialog; the tablet's is drawn here on base.Dialog until ui.Dialog takes a presentation. https://github.com/nsgo-dev/nsgo/issues/742
type Dialog struct {
	Open      func() bool
	OnClose   func()
	OnConfirm func()
	Title     func() string
	Children  []core.Node
	Footer    []core.Node
	TestID    string
}

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	change := func(open bool, _ base.Reason) {
		if !open {
			d.OnClose()
		}
	}
	colors := ui.ColorsOf(cx)
	if !Tablet(cx) {
		// A sheet is the page's grey, so the cards in it stand out as on a page, rounded more than a card at its top.
		// TODO(nsgo): the touch sheet's grabber, padding, height, shadow and motion are fixed (ui/style.go:114-155, sheetPanel and sheetContent) and its title is TypeTitle (ui/dialog.go:64), so the sheet keeps NSGo's 36x5 grabber, 16pt padding and 600 title rather than the design's 38x5 grabber, 6/16/34 padding and 17pt heavy title.
		return ui.Dialog{TestID: d.TestID, Open: d.Open, TitleFunc: d.Title, OnOpenChange: change, OnConfirm: d.OnConfirm, Children: d.Children, Footer: d.Footer,
			StyleFunc: func() core.Style {
				st := core.Style{Background: colors.Background()}
				if ui.Touch(cx) {
					st.CornerRadii = core.Corners{TopLeft: 26, TopRight: 26}
				}
				return st
			}}
	}
	body := append([]core.Node{core.Text{ContentFunc: d.Title, Role: core.RoleHeader, Style: Title.Merge(core.Style{FontWeight: Heavy}), StyleFunc: ui.Colored(colors.Text)}}, d.Children...)
	body = append(body, core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifyEnd, Gap: 8, Margin: core.Edges{Top: 4}}, Children: d.Footer})
	// The design's sheet curve, which the card pops in on.
	curve := core.Curve{X1: 0.32, Y1: 0.72, X2: 0, Y2: 1}
	fade := core.Transition{Duration: 300 * time.Millisecond, Easing: curve, Properties: core.TransitionOpacity}
	pop := core.Transition{Duration: 300 * time.Millisecond, Easing: curve}
	grabber := core.View{Style: core.Style{Width: core.Pt(38), Height: core.Pt(5), CornerRadius: 3, AlignSelf: core.AlignCenter, Margin: core.Edges{Top: 8}, Background: core.RGBA(0x808080, 0.4)}}
	return base.Dialog{TestID: d.TestID, Open: d.Open, OnOpenChange: change, OnConfirm: d.OnConfirm, Animation: core.ModalNone, Role: core.RoleDialog, LabelFunc: d.Title,
		BackdropStyle: core.Style{Background: ui.Black(0.4), Transition: fade}, BackdropStartingStyle: core.Style{Opacity: core.Alpha(0)}, BackdropEndingStyle: core.Style{Opacity: core.Alpha(0)},
		StartingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.94}}, EndingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.98}},
		LayerStyle: core.Style{Padding: core.All(24), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvKeyboard}},
		PopupStyle: core.Style{Width: core.Pt(560), MaxWidth: core.Pct(100), MaxHeight: core.Pct(88), Overflow: core.Hidden, CornerRadius: 24, BorderCurve: core.BorderCurveContinuous, Transition: pop,
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: -10}, Blur: 30, Color: core.RGBA(0x000000, 0.2)}}},
		PopupStyleFunc: func() core.Style {
			return core.Style{Background: colors.Background(), Color: colors.Text()}
		},
		Children: []core.Node{grabber, core.Scroll{Style: core.Style{FlexShrink: 1}, Children: []core.Node{core.View{Style: core.Style{Gap: 16, Padding: core.Edges{Top: 6, Right: 16, Bottom: 34, Left: 16}}, Children: body}}}}}
}
