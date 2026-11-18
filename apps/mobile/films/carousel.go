package films

import (
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// continueMax is how many films the carousel offers to go on with.
const continueMax = 6

// carousel is the films left partway, on this phone or another device, the latest first, in a row that scrolls sideways under its heading.
func (v *view) carousel(cx *core.Scope) core.Node {
	return core.View{TestID: "films-continue", Style: core.Style{Gap: 12}, Children: []core.Node{
		look.SectionTitle(cx, func() string { return locales.FilmsContinue(cx) }),
		core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{FlexGrow: 0, Margin: core.Edges{Right: -16, Left: -16}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.Symmetric(0, 16)}, Children: []core.Node{
				core.Each(v.resumable, func(e catalog.Entry) string { return e.Key() }, func(cx *core.Scope, e func() catalog.Entry, _ func() int) core.Node {
					return v.card(cx, e)
				}),
			}},
		}},
	}}
}

// cardWidth is a card's width: most of a phone's, so the next card shows at the edge.
const cardWidth = 260

// card is a film to go on with: its poster with how far it was watched along its foot and, left on another device, the device's name on it; under it its title and where it was left. Pressed, it plays the film from there, or opens its page to download it first.
func (v *view) card(cx *core.Scope, e func() catalog.Entry) core.Node {
	colors := ui.ColorsOf(cx)
	elsewhere := func() bool { return !e().Position.Mine && e().Position.Device != "" }
	fraction := func() float32 {
		length := time.Duration(e().Film.DurationMS) * time.Millisecond
		if length <= 0 {
			return 0
		}
		return min(float32(e().Position.At)/float32(length), 1)
	}
	meta := func() string {
		at := look.Duration(e().Position.At)
		if elsewhere() {
			return locales.FilmsLeftOn(cx, e().Position.Device, at)
		}
		if length := time.Duration(e().Film.DurationMS) * time.Millisecond; length > 0 {
			return locales.FilmsLeft(cx, look.Duration(length-e().Position.At))
		}
		return locales.FilmsWatchedTo(cx, at)
	}
	id := e().Desktop + "-" + e().Film.ID
	return core.View{TestID: "films-continue-" + id, Role: core.RoleButton, LabelFunc: func() string { return e().Film.Title + ", " + meta() },
		OnPress: func(core.PressEvent) { v.resume(e()) },
		Style:   core.Style{Width: core.Pt(cardWidth), Gap: 8, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden},
				StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} },
				Children: []core.Node{
					core.Image{SourceFunc: poster(cx, v.s, e), ResizeMode: core.Cover, ElementsHidden: true,
						Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}},
					core.Show{When: elsewhere, Then: func(cx *core.Scope) core.Node {
						// The badge is on the picture, so it keeps the dark of a scrim in either appearance.
						return core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(10), Top: core.Pt(10), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5,
							Height: core.Pt(24), Padding: core.Symmetric(0, 9), CornerRadius: 12, Background: core.RGBA(0x000000, 0.55), MaxWidth: core.Pct(90)},
							Children: []core.Node{
								ui.Icon(lucide.Laptop, 13, func() core.Color { return core.RGB(0xFFFFFF) }),
								core.Text{ContentFunc: func() string { return locales.FilmsLeftHere(cx, e().Position.Device) }, MaxLines: 1,
									Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold, Color: core.RGB(0xFFFFFF), FlexShrink: 1})},
							}}
					}},
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Height: core.Pt(4), Background: core.RGBA(0xFFFFFF, 0.3)},
						Children: []core.Node{core.View{StyleFunc: func() core.Style {
							return core.Style{Height: core.Pct(100), Width: core.Pct(fraction() * 100), Background: colors.Accent()}
						}}}},
				}},
			core.View{Style: core.Style{Gap: 2, Padding: core.Symmetric(0, 2)}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return e().Film.Title }, MaxLines: 1, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "films-continue-meta-" + id, ContentFunc: meta, MaxLines: 1, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			}},
		}}
}

// resume plays e's film from where it was last left on any device, once it is on the phone; a film not on the phone yet opens its page, to download it.
func (v *view) resume(e catalog.Entry) {
	k, ok := v.s.Film(e.Key())
	if !ok || !k.Done() {
		v.act.Film(e)
		return
	}
	if !e.Position.Mine && e.Position.At > 0 {
		v.playAt(e.Key(), e.Position.At)
		return
	}
	v.act.Play(e.Key())
}

// playAt plays the film kept under key from at.
func (v *view) playAt(key string, at time.Duration) { v.act.PlayAt(key, at) }
