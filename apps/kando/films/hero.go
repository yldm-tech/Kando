package films

import (
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// onPicture is the text over a poster or the hero, which are a picture or a dark color in either theme.
var onPicture = ns.RGB(0xFFFFFF)

// HeroFilm is the film the hero shows: the one played last that can be continued (ResumeFrom), so a learner coming back is one click from where they were; else the newest one a job finished that was never played; else the newest one a job finished. items is newest first, as the shelf lists them, and a film still in 处理中 is never the hero.
func HeroFilm(a App, items []Film) (Film, bool) {
	var continued Film
	var at time.Time
	found := false
	for _, item := range items {
		if InQueue(item) {
			continue
		}
		if _, ok := ResumeFrom(a, item); !ok {
			continue
		}
		// A film stopped before Kando kept when it played has no time, and comes after those that have one, in the shelf's order.
		if played := a.PlayedAt(item.VideoPath); !found || played.After(at) {
			continued, at, found = item, played, true
		}
	}
	if found {
		return continued, true
	}
	var newest Film
	found = false
	for _, item := range items {
		if item.JobID == "" || item.Stage != core.StageComplete {
			continue
		}
		if a.PlayedAt(item.VideoPath).IsZero() && a.ResumeAt(item.VideoPath) == 0 && a.Furthest(item.VideoPath) == 0 {
			return item, true
		}
		if !found {
			newest, found = item, true
		}
	}
	return newest, found
}

// ResumeFrom is where a film opens or 继续 picks it up: where it stopped, unless that is within 3 seconds of either end. Every way of opening a film but 从头看 and a jump to a line starts there (app.Open).
func ResumeFrom(p Progress, item Film) (time.Duration, bool) {
	at := p.ResumeAt(item.VideoPath)
	if at < watchSlack {
		return 0, false
	}
	if d := p.Duration(item.VideoPath); d > 0 && at > d-watchSlack {
		return 0, false
	}
	return at, true
}

// heroShade is the dark gradient over the hero's poster, from its left, under the text.
var heroShade = ns.Gradient{Kind: ns.GradientLinear, Angle: 90, Stops: []ns.ColorStop{
	{Color: ns.RGBA(0x000000, 0.62), At: ns.Pct(0)},
	{Color: ns.RGBA(0x000000, 0.35), At: ns.Pct(42)},
	{Color: ns.RGBA(0x000000, 0), At: ns.Pct(70)},
}}

// hero is the large card of the film HeroFilm picks: its poster under a dark gradient, its title, its first bilingual line, and play or continue. A click elsewhere on it opens the film, and a right click offers what the film's poster in the grid does. While compact reports true, as while 处理中 has cards under it, it is lower and shows only its kicker, title and buttons, so the queue and the first posters fit a laptop's window.
type hero struct {
	a       App
	item    func() Film
	compact func() bool
}

// The hero's height, full and compact.
const (
	heroHeight        ns.Points = 360
	heroCompactHeight ns.Points = 220
)

// heroQuote is the hero's bilingual line: the translation over the original, both empty until the subtitles have been read.
type heroQuote struct{ translation, original string }

// Render implements core.Node.
func (h hero) Render(cx *ns.Scope) ns.Node {
	a, item := h.a, h.item
	quote := live.Memo(cx, a, func() heroQuote {
		translation, original := a.Quote(item())
		return heroQuote{translation, original}
	})
	quoteBox := ns.Show{When: func() bool { q := quote(); return !h.compact() && (q.translation != "" || q.original != "") }, Then: func(*ns.Scope) ns.Node {
		return ns.View{Style: ns.Style{AlignSelf: ns.AlignStart, Gap: 2, Padding: ns.Symmetric(10, 14), CornerRadius: 12, BorderWidth: 0.5, BorderColor: onPicture.Faded(0.18)}.Merge(frosted(14, ns.RGBA(0x000000, 0.28))),
			Children: []ns.Node{
				ns.Show{When: func() bool { return quote().translation != "" }, Then: func(*ns.Scope) ns.Node {
					return ns.Text{ContentFunc: func() string { return "“" + quote().translation + "”" }, MaxLines: 2, EllipsizeMode: ns.EllipsizeTail,
						Style: ns.Style{FontSize: 15, LineHeight: 21, FontWeight: ui.Semibold, Color: onPicture}}
				}},
				ns.Show{When: func() bool { return quote().original != "" }, Then: func(*ns.Scope) ns.Node {
					return ns.Text{ContentFunc: func() string { return quote().original }, MaxLines: 2, EllipsizeMode: ns.EllipsizeTail,
						Style: ns.Style{FontSize: 12, LineHeight: 15, Color: onPicture}}
				}},
			}}
	}}
	resume := live.Memo(cx, a, func() time.Duration {
		at, _ := ResumeFrom(a, item())
		return at
	})
	// leftOn is the phone the film was left on where it resumes, "" for this computer.
	leftOn := live.Memo(cx, a, func() string { return a.LeftOn(item().VideoPath) })
	// The kicker says whether the film is one to continue, and the phone it was left on, or one finished last.
	kicker := func() string {
		if device := leftOn(); device != "" && resume() > 0 {
			return locales.FilmsHeroKickerContinueOn(cx, device)
		}
		language := item().Language
		// A film whose translation failed shows its original lines only, so it is not said to be bilingual.
		if item().TranslationErr != "" {
			language = ""
		}
		switch {
		case resume() > 0 && language != "":
			return locales.FilmsHeroKickerContinueBilingual(cx, LanguageName(locales.For(cx), language))
		case resume() > 0:
			return locales.FilmsHeroKickerContinue(cx)
		case language != "":
			return locales.FilmsHeroKickerBilingual(cx, LanguageName(locales.For(cx), language))
		}
		return locales.FilmsHeroKicker(cx)
	}
	play := heroButton{main: true, testID: "hero-play",
		label: func() string {
			if at := resume(); at > 0 {
				return locales.FilmsHeroResume(cx, Clock(at))
			}
			return locales.FilmsPlay(cx)
		},
		press: live.Do(a, func() { a.OpenAt(item(), resume()) })}
	// The second button starts the film over while it can resume, and else shows it in the file manager.
	alt := heroButton{testID: "hero-alt",
		label: func() string {
			if resume() > 0 {
				return locales.FilmsHeroFromStart(cx)
			}
			return locales.FilmsShowInFinder(cx, desktop.Platform(cx))
		},
		press: live.Do(a, func() {
			if resume() > 0 {
				a.OpenAt(item(), 0)
				return
			}
			a.Reveal(item().VideoPath)
		})}
	// Its lines are as high as the prototype's, which the browser gives the fonts, so the text block rises from the bottom as far as it does there.
	text := ns.View{PointerEvents: ns.PointerEventsBoxNone, Style: ns.Style{Position: ns.Absolute, Left: ns.Pt(36), Right: ns.Pt(36), Bottom: ns.Pt(34), MaxWidth: ns.Pt(520), Gap: 10}, Children: []ns.Node{
		ns.Text{ContentFunc: kicker, Style: ns.Style{FontSize: 11, LineHeight: 16, FontWeight: ui.Bold, LetterSpacing: 1.2, Color: onPicture.Faded(0.85)}},
		heading(2, ns.Text{ContentFunc: func() string { return filmName(item()) }, MaxLines: 2, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 34, FontWeight: ui.Bold, LineHeight: 40, LetterSpacing: -0.4, Color: onPicture}}),
		ns.Show{When: func() bool { return !h.compact() }, Then: func(*ns.Scope) ns.Node {
			return ns.Text{ContentFunc: live.Func(a, func() string { return heroMeta(locales.For(cx), a, item()) }), Style: ns.Style{FontSize: 13, LineHeight: 16, Color: onPicture.Faded(0.85)}}
		}},
		quoteBox,
		ns.View{Style: ns.Style{Direction: ns.Row, Gap: 10, Wrap: ns.Wrap, Margin: ns.Edges{Top: 6}}, Children: []ns.Node{play, alt}},
	}}
	// The hero is named by its film and has no hint: one would be read once, as the hero is built, and so could say 最近完成 over a kicker that says 继续观看; the kicker, the title and the buttons inside it say the rest.
	box := base.Button{
		TestID:    "films-hero",
		LabelFunc: func() string { return filmName(item()) },
		OnPress:   live.On(a, func(ns.PressEvent) { a.Open(item()) }),
		Style: ns.Style{CornerRadius: 18, Cursor: ns.CursorDefault,
			Shadows: []ns.BoxShadow{{Offset: ns.Point{Y: 12}, Blur: 32, Color: ns.RGBA(0x000000, 0.16)}}, FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: func() ns.Style {
			height := heroHeight
			if h.compact() {
				height = heroCompactHeight
			}
			return ns.Style{Height: ns.Pt(height), Background: posterColor(item().Key)}
		},
		Children: []ns.Node{clip(18,
			cover(live.Func(a, func() ns.ImageSource { return a.Poster(item().VideoPath) })),
			ns.View{PointerEvents: ns.PointerEventsNone, Style: overlay.Merge(ns.Style{Gradients: []ns.Gradient{heroShade}})},
			text,
		)},
	}
	return ns.View{
		OnContextMenu: func(e ns.ContextMenuEvent) { nsdesktop.PopupMenu(cx, filmMenu(cx, a, item()), e.PagePosition, nil) },
		Children:      []ns.Node{box},
	}
}

// heroMeta is the line under the hero's title, in the language of m: where the film comes from, how long it is and how many of its lines are untranslated.
func heroMeta(m locales.Messages, a App, item Film) string {
	parts := []string{SourceLabel(m, item)}
	if d := a.Duration(item.VideoPath); d > 0 {
		parts = append(parts, Clock(d))
	}
	if note := untranslatedNote(m, item); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, " · ")
}

// The hero's main button is white with dark ink; the other is frosted, its rim a light hairline.
var (
	heroMainInk = ns.RGB(0x111114)
	heroShadow  = ns.RGB(0x000000)
)

// heroButton is one of the hero's pill buttons, 34 points high: white with a play glyph for the main one, frosted for the other, a shade darker under the pointer and pressed.
type heroButton struct {
	main   bool
	testID string
	label  func() string
	press  func()
}

// Render implements core.Node.
func (b heroButton) Render(cx *ns.Scope) ns.Node {
	fill, ink := onPicture.Faded(0.2), onPicture
	weight := ui.Medium
	shape := ns.Style{Direction: ns.Row, Height: ns.Pt(34), CornerRadius: 17, Gap: 8, FlexShrink: 0, AlignItems: ns.AlignCenter, Cursor: ns.CursorDefault}
	var children []ns.Node
	if b.main {
		fill, ink, weight = onPicture, heroMainInk, ui.Semibold
		shape.Padding = ns.Edges{Left: 14, Right: 18}
		children = append(children, ns.Icon{SVG: playIcon, Style: ns.Style{Width: ns.Pt(12), Height: ns.Pt(13), Color: ink}})
	} else {
		shape.Padding = ns.Symmetric(0, 16)
		shape.BorderWidth, shape.BorderColor = 0.5, onPicture.Faded(0.35)
		shape = shape.Merge(frosted(16, ns.Color(0)))
	}
	children = append(children, ns.Text{ContentFunc: b.label, MaxLines: 1, Style: ns.Style{FontSize: 14, FontWeight: weight, Color: ink}})
	shape.Background = fill
	shape.Hover = &ns.Style{Background: ui.Mix(fill, heroShadow, 0.05)}
	shape.Active = &ns.Style{Background: ui.Mix(fill, heroShadow, 0.12)}
	shape.FocusVisible = ui.FocusRing(cx, 2)
	return base.Button{TestID: b.testID, LabelFunc: b.label, OnPress: func(ns.PressEvent) { b.press() }, Style: shape, Children: children}
}

// filmName is the film's full name, or its short title when it has none.
func filmName(item Film) string {
	if item.Name != "" {
		return item.Name
	}
	return item.Title
}
