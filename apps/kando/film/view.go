package film

import (
	"path/filepath"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The player's measures, from the prototype (template 401-640).
const (
	// idleAfter is how long the pointer rests while the film plays before the controls fade.
	idleAfter = 2500 * time.Millisecond
	// fadeFor is how long the controls take to fade out or in.
	fadeFor = 350 * time.Millisecond
	// panelWidth is the side panel's, and panelRoom the room the picture, the subtitles and the transport leave for it.
	panelWidth  core.Points = 340
	panelRoom   core.Points = 372
	panelMotion             = 350 * time.Millisecond
)

// onGlass is the text over the player's glass surfaces, which stay dark over any picture.
var onGlass = core.RGB(0xFFFFFF)

// View is the open film over the whole window in dark, the player's theme, which stays dark in the light window. The picture fills the window, or the room left of the side panel while it shows; the title bar, the transport and the subtitles float over it. Item is the open film as the library page finds it by the open key, which changes as its job moves on.
type View struct {
	App  App
	Item func() Item
}

// screen is what the player's parts share: the app, the film, the clock the picture's position follows and whether the controls show.
type screen struct {
	a    App
	cx   *core.Scope
	item func() Item
	// tick moves on each time the system's player says where it is, and at each moment a line of the subtitles starts or ends between two of those reports, so what follows the playhead reads it again then, rather than on every frame.
	tick *core.Signal[struct{}]
	// active is set while the pointer moved over the player in the last idleAfter.
	active *core.Signal[bool]
	// size is the player's size, as its last layout gave it.
	size *core.Signal[core.Size]
	// shown is whether the title bar and the transport show; room is whether the side panel takes its room.
	shown func() bool
	room  func() bool
	// held and scrub are the scrubber's: held while the pointer drags it, scrub the seconds it shows meanwhile.
	held  *core.Signal[bool]
	scrub *core.Signal[float64]
}

// playhead is where the picture is, read again as the clock moves on and after every change of the app's state.
func (s *screen) playhead() time.Duration {
	s.tick.Get()
	s.a.Track()
	return s.a.PlayHead()
}

// shownHead is the playhead, or while the scrubber is held where it would seek.
func (s *screen) shownHead() time.Duration {
	if s.held.Get() {
		return seconds(s.scrub.Get())
	}
	return s.playhead()
}

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	theme.Dark(cx)
	a := v.App
	item := core.NewMemo(cx, v.Item).Get
	s := &screen{a: a, cx: cx, item: item,
		tick:   core.NewSignalFunc(cx, struct{}{}, nil),
		active: core.NewSignal(cx, true),
		size:   core.NewSignal(cx, core.Size{}),
		held:   core.NewSignal(cx, false),
		scrub:  core.NewSignal(cx, 0.0),
	}
	// The controls show for idleAfter after the pointer last moved, and start out showing. A move only notes when it was; the one timer, once it runs, waits the rest of idleAfter when the pointer moved meanwhile, so moving the pointer does not set a timer at each move.
	idle := NewTimer(cx)
	var lastMove time.Time
	var rest func()
	rest = func() {
		if left := idleAfter - cx.Now().Sub(lastMove); left > 0 {
			idle.Set(left, rest)
			return
		}
		s.active.Set(false)
	}
	moved := func() {
		lastMove = cx.Now()
		s.active.Set(true)
		if idle.At().IsZero() {
			idle.Set(idleAfter, rest)
		}
	}
	moved()
	playing := live.Func(a, func() bool { return a.Play() != nil && a.Play().Playing() })
	menus := live.Func(a, func() bool { return *a.ModeMenu() || *a.SpeedMenu() })
	s.shown = core.NewMemo(cx, func() bool { return !playing() || menus() || s.held.Get() || s.active.Get() }).Get
	s.room = live.Memo(cx, a, func() bool { return panelShown(a) })
	return core.View{
		TestID:      "film",
		Style:       core.Style{FlexGrow: 1, MinHeight: core.Pt(0), AlignItems: core.AlignStretch, Background: core.RGB(0x000000), Overflow: core.Hidden},
		OnLayout:    func(b core.Bounds) { s.size.Set(b.Size) },
		OnMouseMove: func(core.MouseEvent) { moved() },
		Children: []core.Node{
			s.stage(),
			s.gradients(),
			s.bigPlay(),
			s.subtitles(),
			s.osd(),
			s.uploadCapsule(),
			s.topBar(),
			s.transport(),
			s.panel(),
		},
	}
}

// panelShown reports whether the side panel shows: when it is on, and always while there is no picture, as for a film still downloading or one whose video is gone. Such a film has no transport, whose toggle is the only control that turns the panel back on, and the panel's subtitle list is all it has to show.
func panelShown(a App) bool {
	return a.Panel() || a.PlayPath() == ""
}

// roomRight is how far from the player's right edge the picture's parts end: the panel's room while it shows, else none.
func (s *screen) roomRight() core.Points {
	if s.room() {
		return panelRoom
	}
	return 0
}

// roomTransition moves a part's frame with the side panel as it comes and goes.
var roomTransition = core.Transition{Duration: panelMotion, Easing: core.EaseInOut, Properties: core.TransitionLayout}

// fadeTransition fades the controls in and out.
var fadeTransition = core.Transition{Duration: fadeFor, Easing: core.EaseInOut, Properties: core.TransitionOpacity}

// faded is the style of a control that fades with the title bar and the transport: transparent and letting the pointer through while they hide.
func (s *screen) faded() core.Style {
	if s.shown() {
		return core.Style{Opacity: core.Alpha(1), Transition: fadeTransition}
	}
	return core.Style{Opacity: core.Alpha(0), Transition: fadeTransition, PointerEvents: core.PointerEventsNone}
}

// togglePlay starts or holds the picture.
func (s *screen) togglePlay() {
	if p := s.a.Play(); p != nil {
		p.Toggle()
	}
}

// stage is the picture, or what stands for it while it opens or cannot play. A click on it starts or holds the film, and a double click enters or leaves full screen: its first click starts or holds the film and its second undoes that, so the film plays on, or stays held, as it enters or leaves full screen. The pointer hides over it with the controls. While the side panel shows, the stage is the room left of it, as the subtitles' and the transport's are, so the panel never hides part of the picture, such as a screencast's text.
func (s *screen) stage() core.Node {
	a, cx := s.a, s.cx
	colors := ui.ColorsOf(cx)
	source := live.Memo(cx, a, func() string { return a.Video().Source })
	poster := live.Memo(cx, a, func() string {
		if a.Play() != nil {
			return ""
		}
		return a.Poster(a.PlayPath())
	})
	note := live.Memo(cx, a, func() stageNoteState {
		item := s.item()
		switch {
		case a.PlayErr() != "":
			return stageNoteState{err: a.PlayErr()}
		case a.PlayPath() != "" && a.Play() == nil:
			return stageNoteState{opening: true}
		case a.PlayPath() == "" && item.VideoPath == "" && item.Detail != "":
			return stageNoteState{detail: item.Detail}
		}
		return stageNoteState{}
	})
	natural := core.NewSignal(cx, core.Size{})
	return core.View{
		TestID: "film-stage", Role: core.RoleGroup, LabelFunc: func() string { return locales.FilmPicture(cx) },
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0)},
		StyleFunc: func() core.Style {
			st := core.Style{Right: core.Pt(s.roomRight()), Transition: roomTransition}
			if !s.shown() {
				st.Cursor = core.CursorNone
			}
			return st
		},
		OnPress: live.On(a, func(e core.PressEvent) {
			if a.Play() == nil {
				return
			}
			s.togglePlay()
			if e.Clicks == 2 {
				a.ToggleFullScreen()
			}
		}),
		Children: []core.Node{
			core.Show{When: func() bool { return poster() != "" }, Then: func(*core.Scope) core.Node {
				// TODO(nsgo#107): show the film's own frames as they decode, as MyGo's VideoPlayer did, in place of its poster while the system's player opens it; the screenshots show this poster in place of the picture, which no system player plays headless https://github.com/nsgo-dev/nsgo/issues/107
				return core.Image{TestID: "film-poster", ResizeMode: core.Cover, Style: fillStyle,
					SourceFunc: func() core.ImageSource { return core.ImageSource{URI: fileURL(poster())} }}
			}},
			core.Show{When: func() bool { return source() != "" }, Then: func(cx *core.Scope) core.Node { return s.video(cx, natural) }},
			core.Show{When: func() bool { return note() != stageNoteState{} }, Then: func(cx *core.Scope) core.Node {
				return core.View{PointerEvents: core.PointerEventsNone,
					Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(80, 24)},
					Children: []core.Node{core.Text{TestID: "film-stage-note",
						ContentFunc: func() string {
							n := note()
							switch {
							case n.err != "":
								return a.ErrText(cx, n.err)
							case n.opening:
								return locales.FilmOpeningVideo(cx)
							}
							return n.detail
						},
						Style: core.Style{TextAlign: core.TextAlignCenter, FontSize: 13, LineHeight: 18},
						StyleFunc: func() core.Style {
							if note().err != "" {
								return core.Style{Color: colors.Text()}
							}
							return core.Style{Color: colors.Muted()}
						}}}}
			}},
		},
	}
}

// stageNoteState is what the stage says in place of the picture: why it would not play, that it opens, or how far the film's job is.
type stageNoteState struct {
	err     string
	opening bool
	detail  string
}

// video is the system's player, filling the stage: it covers the stage with the picture when the two are close in shape and shows it whole otherwise (PictureFit). What it reports goes to the app; its clock moves the player's (tick), between reports at the moments a line of the subtitles starts or ends, so the lines change on time.
func (s *screen) video(cx *core.Scope, natural *core.Signal[core.Size]) core.Node {
	a := s.a
	boundary := NewTimer(cx)
	var nextChange func()
	nextChange = func() {
		boundary.Stop()
		subs := a.Subs()
		p := a.Play()
		if p == nil || !p.Playing() || subs.Key != a.OpenKey() || !subs.Ready {
			return
		}
		after, ok := NextCueChange(subs.Cues, a.PlayHead()-a.Delay())
		if !ok {
			return
		}
		// The film's time runs at its speed, faster or slower than the clock's.
		wall := time.Duration(float64(after) / max(float64(a.Video().Rate), 0.1))
		boundary.Set(wall, func() {
			s.tick.Set(struct{}{})
			nextChange()
		})
	}
	return core.Video{
		TestID: "film-video", Ref: a.VideoRef(),
		Style:      fillStyle,
		SourceFunc: live.Func(a, func() string { return a.Video().Source }),
		PausedFunc: live.Func(a, func() bool { return a.Video().Paused }),
		MutedFunc:  live.Func(a, func() bool { return a.Video().Muted }),
		VolumeFunc: live.Func(a, func() float32 { return a.Video().Volume }),
		RateFunc:   live.Func(a, func() float32 { return a.Video().Rate }),
		ResizeModeFunc: func() core.ResizeMode {
			n, sz := natural.Get(), s.size.Get()
			return PictureFit(sz.Width-s.roomRight(), sz.Height, n.Width, n.Height)
		},
		OnLoad: live.On(a, func(info core.VideoInfo) {
			natural.Set(info.Size)
			a.VideoLoaded(info)
		}),
		OnProgress: func(position, duration time.Duration) {
			if a.VideoProgress(position, duration) {
				a.Changed()
			}
			s.tick.Set(struct{}{})
			nextChange()
		},
		OnEnd:   live.Do(a, a.VideoEnded),
		OnError: live.On(a, a.VideoFailed),
	}
}

// fillStyle fills the parent, over what is in it.
var fillStyle = core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}

// fileURL is the file: URL of path, for an image's source.
func fileURL(path string) string {
	if path == "" {
		return ""
	}
	return "file://" + filepath.ToSlash(path)
}

// gradients darken the top and the bottom of the picture under the title bar and the transport, and fade with them.
func (s *screen) gradients() core.Node {
	band := func(top bool, height core.Points, from, to core.Color) core.Node {
		st := core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(height), Gradients: []core.Gradient{core.LinearGradient(180, from, to)}}
		if top {
			st.Top = core.Pt(0)
		} else {
			st.Bottom = core.Pt(0)
		}
		return core.View{PointerEvents: core.PointerEventsNone, Style: st, StyleFunc: func() core.Style {
			f := s.faded()
			f.PointerEvents = core.PointerEventsNone
			return f
		}}
	}
	return core.View{PointerEvents: core.PointerEventsBoxNone, Style: fillStyle, Children: []core.Node{
		band(true, 120, core.RGBA(0x000000, 0.55), core.RGBA(0x000000, 0)),
		band(false, 220, core.RGBA(0x000000, 0), core.RGBA(0x000000, 0.62)),
	}}
}

// bigPlay is the round button over a held picture, in the middle of the room the side panel leaves.
func (s *screen) bigPlay() core.Node {
	a, cx := s.a, s.cx
	held := live.Memo(cx, a, func() bool { return a.Play() != nil && !a.Play().Playing() })
	return core.Show{When: held, Then: func(cx *core.Scope) core.Node {
		btn := glassButton(cx, glassSpec{fill: core.RGBA(0xFFFFFF, 0.18), blur: 20, ring: 0.35, radius: 38}, core.Style{Width: core.Pt(76), Height: core.Pt(76), Margin: core.Edges{Top: -38}},
			func() string { return locales.FilmPlay(cx) }, "film-big-play", live.Do(a, s.togglePlay),
			core.Icon{SVG: iconBigPlay, Style: core.Style{Width: core.Pt(28), Height: core.Pt(30), Color: onGlass}})
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pct(42), Direction: core.Row, JustifyContent: core.JustifyCenter},
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight()), Transition: roomTransition}
			},
			Children: []core.Node{btn}}
	}}
}
