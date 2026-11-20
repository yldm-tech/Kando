package films

import (
	"context"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// continueMax is how many films the carousel offers to go on with.
const continueMax = 6

// carousel is the films left partway, on this phone or another device, the latest first, in a row that scrolls sideways under its heading, a card at a time.
func (v *view) carousel(cx *core.Scope) core.Node {
	width := cardWidth(cx)
	return core.View{TestID: "films-continue", Style: core.Style{Gap: 10, Margin: core.Edges{Top: 14}}, Children: []core.Node{
		look.SectionTitle(cx, func() string { return locales.FilmsContinue(cx) }),
		core.Scroll{Horizontal: true, HideIndicator: true, Behavior: core.ScrollBehavior{SnapToInterval: width + 12, SnapToAlignment: core.SnapStart},
			Style: core.Style{FlexGrow: 0, Margin: core.Edges{Right: -16, Left: -16}}, Children: []core.Node{
				// The cards' shadow falls under them, so the row leaves it room.
				core.View{Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.Edges{Right: 16, Bottom: 14, Left: 16}}, Children: []core.Node{
					core.Each(v.resumable, func(e catalog.Entry) string { return e.Key() }, func(cx *core.Scope, e func() catalog.Entry, _ func() int) core.Node {
						return v.card(cx, e)
					}),
				}},
			}},
	}}
}

// cardWidth is a card's width: most of a phone's, so the next card shows at the edge; wider on a tablet.
func cardWidth(cx *core.Scope) core.Points {
	if look.Tablet(cx) {
		return 380
	}
	return 300
}

// cue is the line of cues shown at, if any: the one on screen then, else the last one before.
func cue(cues []sync.Cue, at time.Duration) (sync.Cue, bool) {
	ms := at.Milliseconds()
	var got sync.Cue
	found := false
	for _, c := range cues {
		if c.StartMS > ms {
			break
		}
		got, found = c, true
	}
	return got, found
}

// card is a film to go on with, on the dark of a card in either appearance: its poster, darkening to its foot, with the device it was left on at its top, the line it was left at over its foot where the phone keeps its lines, and how far it was watched along its edge; under it its title, where it was left and a play button. Pressed, it plays the film from there, or opens its page to download it first.
func (v *view) card(cx *core.Scope, e func() catalog.Entry) core.Node {
	width := cardWidth(cx)
	image, play := core.Points(168), core.Points(36)
	if look.Tablet(cx) {
		image, play = 214, 40
	}
	white := func(a float32) func() core.Color { return func() core.Color { return core.RGBA(0xFFFFFF, a) } }
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
	// The line it was left at, from the lines kept on the phone; a film only on its computer shows none. The lines are read behind a memo of the film's key, so a saved position, which changes the entry every few seconds while the film plays, does not read them again: the resource loads afresh whenever its source reruns.
	kept := func() bool { k, ok := v.s.Film(e().Key()); return ok && k.Done() }
	src := core.NewMemo(cx, func() string {
		if kept() {
			return e().Key()
		}
		return ""
	})
	lines := core.NewResource(cx, func() (string, bool) { k := src.Get(); return k, k != "" }, func(ctx context.Context, key string, _ core.ResourceInfo) ([]sync.Cue, error) {
		return v.s.Lines(ctx, key)[key], nil
	})
	line := func() (sync.Cue, bool) {
		cues, ok := lines.Get()
		if !ok || !kept() {
			return sync.Cue{}, false
		}
		return cue(cues, e().Position.At)
	}
	id := e().Desktop + "-" + e().Film.ID
	shadow := core.Style{TextShadowColor: core.RGBA(0x000000, 0.8), TextShadowOffset: core.Point{Y: 1}, TextShadowRadius: 3}
	return core.View{TestID: "films-continue-" + id, Role: core.RoleButton, LabelFunc: func() string { return e().Film.Title + ", " + meta() },
		OnPress: func(core.PressEvent) { v.resume(e()) },
		Style: core.Style{Width: core.Pt(width), CornerRadius: 18, BorderCurve: core.BorderCurveContinuous, Background: theme.ContinueCard,
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: 10}, Blur: 26, Color: core.RGBA(0x000000, 0.14)}}, Active: &core.Style{Opacity: core.Alpha(0.8)}},
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pt(image), CornerRadii: core.Corners{TopLeft: 18, TopRight: 18}, Overflow: core.Hidden, Background: core.RGB(0x2C2C2E)},
				Children: []core.Node{
					core.Image{SourceFunc: poster(cx, v.s, e), ResizeMode: core.Cover, ElementsHidden: true,
						Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}},
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0),
						Gradients: []core.Gradient{{Kind: core.GradientLinear, Angle: 180, Stops: []core.ColorStop{{Color: core.RGBA(0x000000, 0), At: core.Pct(40)}, {Color: core.RGBA(0x000000, 0.7), At: core.Pct(100)}}}}}},
					core.Show{When: elsewhere, Then: func(cx *core.Scope) core.Node {
						// The pill is on the picture, so it keeps the dark of a scrim in either appearance, frosting the picture behind it.
						return core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(12), Top: core.Pt(12), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5,
							Height: core.Pt(24), Padding: core.Symmetric(0, 10), CornerRadius: 12, Background: core.RGBA(0x000000, 0.5), BackdropFilters: []core.Filter{core.Blur(10)}, MaxWidth: core.Pct(90)},
							Children: []core.Node{
								ui.Icon(lucide.Laptop, 13, white(1)),
								core.Text{ContentFunc: func() string { return locales.FilmsLeftHere(cx, e().Position.Device) }, MaxLines: 1,
									Style: core.Style{FontSize: 12, LineHeight: 15, FontWeight: core.FontWeightBold, Color: core.RGB(0xFFFFFF), FlexShrink: 1}},
							}}
					}},
					core.Show{When: func() bool { _, ok := line(); return ok }, Then: func(cx *core.Scope) core.Node {
						return core.View{TestID: "films-continue-line-" + id, AccessibilityElementsHidden: func() bool { return true },
							Style: core.Style{Position: core.Absolute, Left: core.Pt(14), Right: core.Pt(14), Bottom: core.Pt(12), AlignItems: core.AlignCenter, Gap: 2},
							Children: []core.Node{
								// A line not translated shows its original as the large line, alone.
								core.Text{ContentFunc: func() string {
									c, _ := line()
									if c.Translation != "" {
										return c.Translation
									}
									return c.Original
								}, MaxLines: 1,
									Style: shadow.Merge(core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, Color: core.RGB(0xFFFFFF), TextAlign: core.TextAlignCenter})},
								core.Show{When: func() bool { c, _ := line(); return c.Translation != "" }, Then: func(cx *core.Scope) core.Node {
									return core.Text{ContentFunc: func() string { c, _ := line(); return c.Original }, MaxLines: 1,
										Style: shadow.Merge(core.Style{FontSize: 12, LineHeight: 15, Color: theme.SubtitleYellow, TextAlign: core.TextAlignCenter})}
								}},
							}}
					}},
				}},
			core.View{Style: core.Style{Height: core.Pt(3), Background: core.RGBA(0xFFFFFF, 0.18)},
				Children: []core.Node{core.View{StyleFunc: func() core.Style {
					return core.Style{Height: core.Pct(100), Width: core.Pct(fraction() * 100), Background: theme.PlayerBlue}
				}}}},
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 11, Right: 12, Bottom: 12, Left: 14}}, Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return e().Film.Title }, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, Color: core.RGB(0xFFFFFF)}},
					core.Text{TestID: "films-continue-meta-" + id, ContentFunc: meta, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, Color: core.RGBA(0xFFFFFF, 0.6)}},
				}},
				core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Width: core.Pt(play), Height: core.Pt(play), CornerRadius: play / 2, Background: core.RGB(0xFFFFFF),
					AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0, Padding: core.Edges{Left: 2}},
					Children: []core.Node{core.Icon{SVG: playIcon, Style: core.Style{Width: core.Pt(play * 14 / 36), Height: core.Pt(play * 14 / 36), Color: core.RGB(0x000000)}}}},
			}},
		}}
}

// playIcon is the design's solid play triangle.
var playIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 14 14"><path d="M3 1.6v10.8a.8.8 0 0 0 1.2.7l9-5.4a.8.8 0 0 0 0-1.4l-9-5.4A.8.8 0 0 0 3 1.6z" fill="currentColor"/></svg>`))

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
