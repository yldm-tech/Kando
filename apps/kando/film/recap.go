package film

import (
	"slices"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Recapper is the 前情提要 card of a film opened where it stopped last time (design Main, 前情提要): what was watched of it, summed up, and where to play from.
type Recapper interface {
	// Recap is the card, the zero Recap while it does not show.
	Recap() Recap
	// CloseRecap closes the card, as its × and Escape do; ContinueRecap plays the film from where it stopped, StartOver from its start, and RecapJump from the moment of item i. Each closes the card.
	CloseRecap()
	ContinueRecap()
	StartOver()
	RecapJump(i int)
}

// Recap is the card as it shows: where the film stopped last time, At, and Items, the moments of the part watched with what happened there, or while Busy none, as the AI service is asked. Excerpts marks items that are lines watched rather than summaries. Note is the card's footnote, which says what made the items, in the language the interface shows under the scope it is given.
type Recap struct {
	Open     bool
	At       time.Duration
	Items    []RecapItem
	Busy     bool
	Excerpts bool
	Note     func(*core.Scope) string
}

// RecapItem is a moment of the part watched, in the subtitles' time, and what happened there.
type RecapItem struct {
	At   time.Duration
	Text string
}

// recapGlass is the card's glass (design Main 674: rgba(28,28,32,0.82), blur 28, radius 18).
var recapGlass = glassSpec{fill: core.RGBA(0x1C1C20, 0.82), blur: 28, ring: 0.2, radius: 18}

// recapCard is the 前情提要 card over the top left of the picture while the film opened held where it stopped last time (App.Recap): 上次看到 with the × that closes it, 前情 and its items, each playing from its moment at a click, or that they are being summed up; then 从 X 继续 and 从头看, and the footnote saying what made the items. Escape closes it.
func (s *screen) recapCard() core.Node {
	a, cx := s.a, s.cx
	open := live.Memo(cx, a, func() bool { return a.Recap().Open })
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		r := live.Func(a, a.Recap)
		cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, live.Do(a, a.CloseRecap))
		at := func() string { return cueClock(r().At) }
		muted := core.RGBA(0xFFFFFF, 0.6)
		head := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.Text{TestID: "film-recap-at", ContentFunc: func() string { return locales.RecapLastAt(cx, at()) }, Role: core.RoleHeader,
				Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, LineHeight: 19, FontWeight: core.FontWeightBold, Color: onGlass}},
			barGlyph(cx, "×", func() string { return locales.RecapClose(cx) }, "escape", "film-recap-close", live.Do(a, a.CloseRecap)),
		}}
		items := core.NewMemoFunc(cx, live.Func(a, func() []RecapItem { return r().Items }), slices.Equal[[]RecapItem]).Get
		list := core.Dynamic(func(cx *core.Scope) core.Node {
			rows := items()
			if len(rows) == 0 {
				return core.Show{When: func() bool { return r().Busy }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "film-recap-busy", ContentFunc: func() string { return locales.RecapBusy(cx) }, LiveRegion: core.LiveRegionPolite,
						Style: core.Style{FontSize: 13, LineHeight: 18, Color: muted}}
				}}
			}
			nodes := make([]core.Node, 0, len(rows))
			for i, it := range rows {
				nodes = append(nodes, recapRow(cx, it, live.Do(a, func() { a.RecapJump(i) })))
			}
			return core.View{Style: core.Style{Gap: 2}, Children: nodes}
		})
		continueLabel := func() string { return locales.RecapContinue(cx, at()) }
		startLabel := func() string { return locales.RecapStartOver(cx) }
		buttons := core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8, Margin: core.Edges{Top: 4}}, Children: []core.Node{
			base.Button{TestID: "film-recap-continue", LabelFunc: continueLabel, OnPress: live.On(a, func(core.PressEvent) { a.ContinueRecap() }),
				Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
				Style: core.Style{Direction: core.Row, Gap: 6, Height: core.Pt(30), Padding: core.Symmetric(0, 14), AlignItems: core.AlignCenter, CornerRadius: 15, Background: core.RGB(0xFFFFFF),
					Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.88)}, FocusVisible: ui.FocusRing(cx, 2)},
				Children: []core.Node{
					core.Icon{SVG: iconPlay, Style: core.Style{Width: core.Pt(10), Height: core.Pt(10), Color: core.RGB(0x1C1C20)}},
					core.Text{ContentFunc: continueLabel, MaxLines: 1, PointerEvents: core.PointerEventsNone, Style: core.Style{FontSize: 12, LineHeight: 16, FontWeight: core.FontWeightBold, Color: core.RGB(0x1C1C20)}},
				}},
			base.Button{TestID: "film-recap-start", LabelFunc: startLabel, OnPress: live.On(a, func(core.PressEvent) { a.StartOver() }),
				Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
				Style: core.Style{Height: core.Pt(30), Padding: core.Symmetric(0, 14), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: 15, Background: core.RGBA(0xFFFFFF, 0.14),
					Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.22)}, FocusVisible: ui.FocusRing(cx, 2)},
				Children: []core.Node{core.Text{ContentFunc: startLabel, MaxLines: 1, PointerEvents: core.PointerEventsNone, Style: core.Style{FontSize: 12, LineHeight: 16, Color: onGlass}}}},
		}}
		note := core.Show{When: func() bool { return r().Note != nil }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "film-recap-note", ContentFunc: func() string { return r().Note(cx) }, Style: core.Style{FontSize: 11, LineHeight: 15, Color: core.RGBA(0xFFFFFF, 0.5)}}
		}}
		card := core.View{TestID: "film-recap", Role: core.RoleGroup, LabelFunc: func() string { return locales.RecapCard(cx) },
			Style: recapGlass.style().Merge(core.Style{Width: core.Pct(100), MaxWidth: core.Pt(400), Gap: 8, Padding: core.Symmetric(14, 16),
				Shadows: append(recapGlass.style().Shadows, core.BoxShadow{Offset: core.Point{Y: 18}, Blur: 46, Color: core.RGBA(0x000000, 0.4)})}),
			Children: []core.Node{
				head,
				core.Text{ContentFunc: func() string { return locales.RecapHeading(cx) }, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold, Color: muted}},
				list, buttons, note,
			}}
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(24), Top: core.Pt(72), Direction: core.Row},
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight() + 24), Transition: roomTransition}
			},
			Children: []core.Node{card}}
	}}
}

// recapRow is an item of the recap: its moment in a narrow column and what happened there, a button that plays from the moment.
func recapRow(cx *core.Scope, it RecapItem, press func()) core.Node {
	label := func() string { return cueClock(it.At) + " " + it.Text }
	return base.Button{TestID: "film-recap-item", LabelFunc: label, OnPress: func(core.PressEvent) { press() },
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(3, 4), CornerRadius: 8, AlignItems: core.AlignStart,
			Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.1)}, FocusVisible: ui.FocusRing(cx, -2)},
		Children: []core.Node{
			core.Text{Content: cueClock(it.At), PointerEvents: core.PointerEventsNone,
				Style: core.Style{Width: core.Pt(38), FlexShrink: 0, FontSize: 11, LineHeight: 18, Color: core.RGBA(0xFFFFFF, 0.6), FontVariant: core.FontTabularNums}},
			core.Text{Content: it.Text, PointerEvents: core.PointerEventsNone, Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 13, LineHeight: 18, Color: onGlass}},
		}}
}
