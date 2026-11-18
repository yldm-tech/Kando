package films

import (
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Handoff is where a paired phone last left a film of the open library, which the bar under the top bar offers to continue from here: the film, the phone's name, where it was left and when. A zero Film.Key is none to offer.
type Handoff struct {
	Film   Film
	Device string
	At     time.Duration
	When   time.Time
}

// handoffBar is the bar under the message bar that says where a phone left a film, with 从 m:ss 接着看, which opens it there, and ×, which closes the bar until a phone leaves a film again (CloseHandoff).
type handoffBar struct{ a App }

// Render implements core.Node.
func (h handoffBar) Render(cx *core.Scope) core.Node {
	a := h.a
	look := live.Memo(cx, a, a.Handoff)
	return core.Show{When: func() bool { return look().Film.Key != "" }, Then: func(cx *core.Scope) core.Node {
		colors, k := ui.ColorsOf(cx), theme.Of(cx)
		title := core.Text{TestID: "handoff-title", ContentFunc: func() string {
			l := look()
			return locales.FilmsHandoff(cx, l.Device, filmName(l.Film), Clock(l.At))
		}, MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)}
		when := core.Text{ContentFunc: func() string { return locales.FilmsHandoffWhen(cx, ago(cx, look().When)) }, MaxLines: 1,
			Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)}
		text := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{title, when}}
		dot := core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4, FlexShrink: 0}, StyleFunc: ui.Filled(colors.Accent)}
		resume := core.Dynamic(func(cx *core.Scope) core.Node {
			return ui.Button{Text: locales.FilmsHandoffResume(cx, Clock(look().At)), Size: ui.SizeSmall, TestID: "handoff-resume",
				OnPress: live.Do(a, func() {
					l := look()
					a.CloseHandoff()
					a.OpenAt(l.Film, l.At)
				})}
		})
		close := base.Button{TestID: "handoff-close", LabelFunc: func() string { return locales.FilmsCloseNotice(cx) },
			OnPress: live.On(a, func(core.PressEvent) { a.CloseHandoff() }),
			Style: core.Style{Width: core.Pt(24), Height: core.Pt(24), CornerRadius: 12, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Cursor: core.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
			StyleFunc: func() core.Style { return core.Style{Hover: &core.Style{Background: k.Shade(0.06)}} },
			Children:  []core.Node{core.Text{Content: "×", Style: core.Style{FontSize: 15}, StyleFunc: ui.Colored(colors.Muted)}}}
		return core.View{
			TestID:     "films-handoff",
			LiveRegion: core.LiveRegionPolite,
			Role:       core.RoleGroup,
			LabelFunc:  func() string { l := look(); return locales.FilmsHandoff(cx, l.Device, filmName(l.Film), Clock(l.At)) },
			Style: core.Style{Direction: core.Row, MinHeight: core.Pt(52), Margin: core.Edges{Bottom: 14}, Padding: core.Edges{Top: 8, Right: 10, Bottom: 8, Left: 14}, Gap: 12,
				AlignItems: core.AlignCenter, CornerRadius: 14, BorderWidth: 0.5},
			StyleFunc: func() core.Style { return core.Style{Background: k.Panel(), BorderColor: k.Shade(0.08)} },
			Children:  []core.Node{dot, text, resume, close},
		}
	}}
}

// ago is how long before now at was, in the interface's language.
func ago(cx *core.Scope, at time.Time) string {
	d := time.Since(at)
	switch {
	case d < time.Minute:
		return locales.PhoneSyncAgoNow(cx)
	case d < time.Hour:
		return locales.PhoneSyncAgoMinutes(cx, int(d/time.Minute))
	case d < 24*time.Hour:
		return locales.PhoneSyncAgoHours(cx, int(d/time.Hour))
	}
	return locales.PhoneSyncAgoDays(cx, int(d/(24*time.Hour)))
}
