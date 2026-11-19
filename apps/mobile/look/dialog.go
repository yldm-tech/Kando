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
	if !Tablet(cx) {
		return ui.Dialog{TestID: d.TestID, Open: d.Open, TitleFunc: d.Title, OnOpenChange: change, OnConfirm: d.OnConfirm, Children: d.Children, Footer: d.Footer}
	}
	colors := ui.ColorsOf(cx)
	body := append([]core.Node{core.Text{ContentFunc: d.Title, Role: core.RoleHeader, Style: Title.Merge(core.Style{FontWeight: core.FontWeightBold}), StyleFunc: ui.Colored(colors.Text)}}, d.Children...)
	body = append(body, core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifyEnd, Gap: 8, Margin: core.Edges{Top: 4}}, Children: d.Footer})
	fade := core.Transition{Duration: 180 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionOpacity}
	pop := core.Transition{Duration: 180 * time.Millisecond, Easing: core.EaseOut}
	return base.Dialog{TestID: d.TestID, Open: d.Open, OnOpenChange: change, OnConfirm: d.OnConfirm, Animation: core.ModalNone, Role: core.RoleDialog, LabelFunc: d.Title,
		BackdropStyle: core.Style{Background: ui.Black(0.4), Transition: fade}, BackdropStartingStyle: core.Style{Opacity: core.Alpha(0)}, BackdropEndingStyle: core.Style{Opacity: core.Alpha(0)},
		StartingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.96}}, EndingStyle: core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.98}},
		LayerStyle: core.Style{Padding: core.All(24), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvKeyboard}},
		PopupStyle: core.Style{Width: core.Pt(560), MaxWidth: core.Pct(100), MaxHeight: core.Pct(100), Overflow: core.Hidden, CornerRadius: 24, BorderCurve: core.BorderCurveContinuous, Transition: pop},
		PopupStyleFunc: func() core.Style {
			return ui.FinOf(cx).Dialog().Over(core.Style{Color: colors.Text()})
		},
		Children: []core.Node{core.Scroll{Style: core.Style{FlexShrink: 1}, Children: []core.Node{core.View{Style: core.Style{Gap: 16, Padding: core.All(24)}, Children: body}}}}}
}
