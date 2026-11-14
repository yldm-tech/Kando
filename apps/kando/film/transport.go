package film

import (
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// transportGlass is the glass of the capsule along the bottom of the picture.
var transportGlass = glassSpec{fill: core.RGBA(0x1E1E22, 0.5), blur: 28, ring: 0.22, radius: 24}

// transport is the glass capsule along the bottom of the picture: back and forward 5 seconds, play or hold, the time played, the scrubber, the time left, the volume, the speed, the line loop, the subtitle mode, the side panel and full screen. It shows once a video is opening and fades with the title bar.
func (s *screen) transport() core.Node {
	a := s.a
	return core.Show{When: live.Func(a, func() bool { return a.PlayPath() != "" }), Then: func(cx *core.Scope) core.Node {
		clock := func(d func() time.Duration, prefix string, faint bool) core.Node {
			st := core.Style{FontSize: 12, LineHeight: 16, FontVariant: core.FontTabularNums, Color: onGlass, MinWidth: core.Pt(32), FlexShrink: 0}
			if faint {
				st.Opacity = core.Alpha(0.75)
			}
			return core.Text{ContentFunc: func() string { return prefix + cueClock(d()) }, MaxLines: 1, Style: st}
		}
		left := func() time.Duration {
			a.Track()
			return max(a.PlayLength()-s.shownHead(), 0)
		}
		back := func() string { return locales.FilmSkipBack(cx, 5) }
		forward := func() string { return locales.FilmSkipForward(cx, 5) }
		fullLabel := func() string { return locales.FilmFullScreen(cx) }
		panelLabel := func() string { return locales.FilmSidebar(cx) }
		return core.View{
			TestID: "film-transport",
			Style: transportGlass.style().Merge(core.Style{Position: core.Absolute, Left: core.Pt(20), Bottom: core.Pt(18), Direction: core.Row, Gap: 10,
				Padding: core.Edges{Top: 8, Right: 10, Bottom: 8, Left: 12}, AlignItems: core.AlignCenter}).
				Merge(core.Style{Shadows: append(transportGlass.style().Shadows, core.BoxShadow{Offset: core.Point{Y: 10}, Blur: 30, Color: core.RGBA(0x000000, 0.3)})}),
			StyleFunc: func() core.Style {
				st := s.faded()
				st.Right = core.Pt(20)
				if s.room() {
					st.Right = core.Pt(panelRoom + 16)
				}
				st.Transition.Properties |= core.TransitionLayout
				st.Transition.Duration = panelMotion
				return st
			},
			Children: []core.Node{
				row(2,
					skipButton(cx, iconBack5, back, withKey(cx, back, "left"), "film-back5", live.Do(a, func() { a.Skip(-5 * time.Second) })),
					s.playButton(cx),
					skipButton(cx, iconForward5, forward, withKey(cx, forward, "right"), "film-forward5", live.Do(a, func() { a.Skip(5 * time.Second) })),
				),
				clock(s.shownHead, "", false),
				s.scrubber(cx),
				clock(left, "-", true),
				s.volume(cx),
				row(2,
					s.speedMenu(cx),
					s.loopToggle(cx),
					s.modeMenu(cx),
					glyphToggle(cx, svg(iconPanel), panelLabel, withKey(cx, panelLabel, "mod+shift+t"), 34, 30, 18, 15, "film-panel", live.Func(a, a.Panel), live.Do(a, a.TogglePanel)),
					glyphButton(cx, svg(iconFullScreen), fullLabel, func() string { return fullLabel() + " " + cx.KeyLabel("f") }, 34, 30, 16, 16, "film-full", live.Do(a, a.ToggleFullScreen)),
				),
			},
		}
	}}
}

// row lays children out in a row gap apart, at their own size.
func row(gap core.Points, children ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, Gap: gap, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: children}
}

// skipButton is a 5-second skip: the prototype's round arrow with a 5 inside it, in a 32-point button.
func skipButton(cx *core.Scope, icon *core.SVG, label, tooltip func() string, id string, press func()) core.Node {
	b := glyphButton(cx, svg(icon), label, tooltip, 32, 32, 20, 20, id, press)
	// The prototype's 5 sits on the arrow's middle, its baseline 13.2 down the 20-point glyph.
	b.Children = append(b.Children, core.Text{Content: "5", Role: core.RoleNone, PointerEvents: core.PointerEventsNone,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(14.2), FontSize: 6.5, LineHeight: 6.5, FontWeight: core.FontWeightBold, TextAlign: core.TextAlignCenter, Color: onGlass}})
	return b
}

// playButton starts or holds the film. While the video opens it waits, named for that, so that 播放 shows only once there is a picture to play.
func (s *screen) playButton(cx *core.Scope) core.Node {
	a := s.a
	state := live.Memo(cx, a, func() int {
		switch p := a.Play(); {
		case p == nil:
			return 0
		case p.Playing():
			return 2
		}
		return 1
	})
	label := func() string {
		switch state() {
		case 0:
			return locales.FilmOpening(cx)
		case 2:
			return locales.FilmPause(cx)
		}
		return locales.FilmPlay(cx)
	}
	icon := func() *core.SVG {
		if state() == 2 {
			return iconPause
		}
		return iconPlay
	}
	tooltip := func() string {
		if state() == 0 {
			return label()
		}
		return label() + " " + locales.FilmKeySpace(cx)
	}
	b := glyphButton(cx, icon, label, tooltip, 36, 36, 20, 20, "film-play", live.Do(a, s.togglePlay))
	b.Disabled = func() bool { return state() == 0 }
	return b
}

// loopToggle plays the line playing over and over, lit while it does. It waits for the film's subtitle lines, which it loops.
func (s *screen) loopToggle(cx *core.Scope) core.Node {
	a := s.a
	label := func() string { return locales.FilmLoopLine(cx) }
	b := glyphToggle(cx, svg(iconLoopLine), label, withKey(cx, label, "l"), 34, 30, 18, 18, "film-loop", live.Func(a, a.LoopLine), live.Do(a, a.ToggleLoopLine))
	b.Disabled = live.Func(a, func() bool {
		subs := a.Subs()
		return subs.Key != a.OpenKey() || !subs.Ready || len(subs.Cues) == 0
	})
	return b
}
