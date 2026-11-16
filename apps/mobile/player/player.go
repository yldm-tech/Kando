// Package player plays a film kept on the phone, in the dark whatever the system's appearance: the video with its bilingual subtitles drawn over it as the mode picks their lines (original, translation or both, Kando's shared subtitle model), the transport with the line before and after, looping a line and the speed, and the list of lines, which follows the one playing and plays from the one tapped. Where it was left is kept with the film.
//
// The layout follows the window: on a phone held upright the video sits on top, the transport and the lines under it; turned sideways the video fills the screen and a tap shows the controls over it; on a tablet held sideways the lines take a column beside the video.
package player

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Speeds are the speeds the player offers.
var Speeds = []float32{0.5, 0.75, 1, 1.25, 1.5}

// Modes are the subtitle modes the player offers, in the order its menu lists them.
var Modes = []string{kcore.SubBilingual, kcore.SubOriginal, kcore.SubTranslation}

// The settings the player keeps in the app's storage, for every film: the subtitle mode and the speed.
const (
	modeKey  = "player.mode"
	speedKey = "player.speed"
)

// saveEvery is how often, in the film's time, the position playing is kept.
const saveEvery = 5 * time.Second

// layout is how the player lays itself out in the window.
type layout uint8

const (
	// upright is a phone held upright, or a narrow window: video, transport, lines.
	upright layout = iota
	// full is a phone on its side: the video fills the screen, the controls over it.
	full
	// side is a tablet on its side: the video and transport, the lines beside them.
	side
)

func layoutOf(w core.Size) layout {
	switch {
	case w.Width > w.Height && w.Height < 600:
		return full
	case w.Width > w.Height && w.Width >= 900:
		return side
	}
	return upright
}

// Page plays the film kept under key.
func Page(cx *core.Scope, s *session.Session, key string, back func()) base.NavPage {
	theme.Dark(cx)
	f, _ := s.Film(key)
	p := newPlay(cx, s, key)
	shape := core.NewMemo(cx, func() layout { return layoutOf(cx.Env().Window()) })
	controls := core.NewSignal(cx, true)

	video := core.Video{TestID: "player-video", Ref: p.ref, Label: f.Film.Title,
		Source:           filepath.Join(s.Store().FilmDir(f.Desktop, f.Film.ID), f.Video),
		PausedFunc:       func() bool { return !p.playing.Get() },
		RateFunc:         p.speed,
		ProgressInterval: 100 * time.Millisecond,
		AudioSession:     core.AudioSessionPlayback,
		ResizeMode:       core.Contain,
		Style:            fill,
		OnLoad: func(info core.VideoInfo) {
			p.length.Set(info.Duration)
			if info.Size.Width > 0 && info.Size.Height > 0 {
				p.aspect.Set(float32(info.Size.Width / info.Size.Height))
			}
			if at := time.Duration(f.PositionMS) * time.Millisecond; at > 0 && (info.Duration == 0 || at < info.Duration-2*time.Second) {
				p.seek(at)
			}
		},
		OnProgress: p.progress,
		OnEnd:      func() { p.playing.Set(false); p.save() },
		OnError:    func(error) { p.failed.Set(true) },
		OnPress: func(core.PressEvent) {
			if shape.Get() == full {
				controls.Update(func(on bool) bool { return !on })
			}
		},
	}
	failed := core.Show{When: p.failed.Get, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24)}), Children: []core.Node{
			core.Text{TestID: "player-failed", ContentFunc: func() string { return locales.PlayerCantPlay(cx) }, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter, Color: core.RGB(0xFFFFFF)})},
		}}
	}}
	overlaid := func() bool { return shape.Get() != full || controls.Get() }
	stage := core.View{TestID: "player-stage", Style: core.Style{Background: core.RGB(0x000000), Overflow: core.Hidden},
		StyleFunc: func() core.Style {
			if shape.Get() == full {
				return core.Style{FlexGrow: 1, FlexShrink: 1}
			}
			return core.Style{Width: core.Pct(100), AspectRatio: max(p.aspect.Get(), 1)}
		},
		Children: []core.Node{video, subtitles(cx, p, shape.Get, overlaid), failed}}

	bar := topBar(cx, f.Film.Title, back)
	transport := transport(cx, p)
	// On its side the phone shows the top bar and the transport over the video while the controls show.
	over := func(top bool) func() core.Style {
		return func() core.Style {
			if shape.Get() != full {
				return core.Style{}
			}
			st := core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Background: core.RGBA(0x000000, 0.55), ZIndex: 2,
				PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea, Right: core.EnvSafeArea}}
			if top {
				st.Top = core.Pt(0)
			} else {
				st.Bottom = core.Pt(0)
				st.PaddingEnv.Bottom = core.EnvSafeArea
			}
			if !controls.Get() {
				st.Display = core.DisplayNone
			}
			return st
		}
	}
	main := core.View{TestID: "player-main", StyleFunc: func() core.Style {
		st := core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}
		if shape.Get() == upright {
			st.FlexGrow, st.FlexShrink = 0, 0
		}
		return st
	}, Children: []core.Node{
		core.View{StyleFunc: over(true), Children: []core.Node{bar}},
		stage,
		core.View{StyleFunc: over(false), Children: []core.Node{transport}},
	}}
	list := core.View{TestID: "player-lines-pane", StyleFunc: func() core.Style {
		switch shape.Get() {
		case full:
			return core.Style{Display: core.DisplayNone}
		case side:
			return core.Style{Width: core.Pt(380), BorderWidth: 0, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea}}
		}
		return core.Style{FlexGrow: 1, FlexShrink: 1, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}
	}, Children: []core.Node{lines(cx, p)}}

	colors := ui.ColorsOf(cx)
	root := core.View{TestID: "player", Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		StyleFunc: func() core.Style {
			st := core.Style{Background: colors.Background()}
			switch shape.Get() {
			case side:
				st.Direction = core.Row
				st.PaddingEnv = core.EnvEdges{Top: core.EnvSafeArea, Left: core.EnvSafeArea}
			case upright:
				st.PaddingEnv = core.EnvEdges{Top: core.EnvSafeArea}
			default:
				st.Background = core.RGB(0x000000)
			}
			return st
		},
		Children: []core.Node{main, list}}
	return base.NavPage{TestID: "player-page", HeaderHidden: true,
		StatusBarFunc: func() core.StatusBarSetting {
			hidden := core.ToggleOff
			if shape.Get() == full && !controls.Get() {
				hidden = core.ToggleOn
			}
			return core.StatusBarSetting{Foreground: core.StatusBarLight, Hidden: hidden}
		},
		Children: []core.Node{root}}
}

// fill fills the parent, over what is in it.
var fill = core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}

// play is the player's state on the engine's goroutine.
type play struct {
	cx      *core.Scope
	s       *session.Session
	key     string
	ref     *core.Ref
	cues    *core.Resource[[]cues.Cue]
	playing *core.Signal[bool]
	at      *core.Signal[time.Duration]
	length  *core.Signal[time.Duration]
	aspect  *core.Signal[float32]
	failed  *core.Signal[bool]
	// loop is the line looped, -1 for none.
	loop  *core.Signal[int]
	mode  *core.Persisted[string]
	rate  *core.Persisted[float32]
	saved time.Duration
	// current is the line showing, -1 for none.
	current *core.Memo[int]
}

func newPlay(cx *core.Scope, s *session.Session, key string) *play {
	f, _ := s.Film(key)
	p := &play{cx: cx, s: s, key: key, ref: &core.Ref{},
		playing: core.NewSignal(cx, true),
		at:      core.NewSignal(cx, time.Duration(f.PositionMS)*time.Millisecond),
		length:  core.NewSignal(cx, time.Duration(f.Film.DurationMS)*time.Millisecond),
		aspect:  core.NewSignal(cx, float32(16.0/9)),
		failed:  core.NewSignal(cx, false),
		loop:    core.NewSignal(cx, -1),
		mode:    core.NewPersisted(cx, modeKey, kcore.SubBilingual, core.PersistOptions{}),
		rate:    core.NewPersisted(cx, speedKey, float32(1), core.PersistOptions{}),
		saved:   time.Duration(f.PositionMS) * time.Millisecond,
	}
	p.cues = core.NewResource(cx, func() (string, bool) { return key, true }, func(_ context.Context, key string, _ core.ResourceInfo) ([]cues.Cue, error) {
		list, _, err := s.Store().Cues(f.Desktop, f.Film.ID)
		return cues.FromSync(list), err
	})
	p.current = core.NewMemo(cx, func() int { return cues.Index(p.list(), p.at.Get()) })
	// The screen stays on while the film plays.
	var release func()
	cx.Effect(func() {
		if p.playing.Get() && release == nil {
			release = cx.KeepAwake(locales.PlayerKeepAwake(cx), core.KeepAwakeOptions{Display: true})
		} else if !p.playing.Get() && release != nil {
			release()
			release = nil
		}
	})
	cx.Cleanup(p.save)
	return p
}

// list is the film's lines, none until they are read.
func (p *play) list() []cues.Cue {
	v, _ := p.cues.Get()
	return v
}

// speed is the speed playing.
func (p *play) speed() float32 { return p.rate.Get() }

// seek plays from at.
func (p *play) seek(at time.Duration) {
	p.ref.Seek(at)
	p.at.Set(at)
}

// progress hears the video's position as it plays: it moves the playhead, takes a looped line back to its start, and keeps the position now and then.
func (p *play) progress(at, length time.Duration) {
	if length > 0 {
		p.length.Set(length)
	}
	p.at.Set(at)
	if i := p.loop.GetUntracked(); i >= 0 && i < len(p.list()) && cues.LoopBack(p.list(), i, at, p.length.GetUntracked()) {
		start, _ := cues.Span(p.list(), i, p.length.GetUntracked())
		p.seek(start)
		return
	}
	if d := at - p.saved; d >= saveEvery || d <= -saveEvery {
		p.save()
	}
}

// save keeps the position playing with the film.
func (p *play) save() {
	p.saved = p.at.GetUntracked()
	p.s.SavePosition(p.key, p.saved)
}

// toggle starts or holds the film.
func (p *play) toggle() {
	p.playing.Update(func(on bool) bool { return !on })
	if !p.playing.GetUntracked() {
		p.save()
	}
}

// step plays from the start of line i, which the loop moves to while it is on.
func (p *play) step(i int) {
	if i < 0 || i >= len(p.list()) {
		return
	}
	if p.loop.GetUntracked() >= 0 {
		p.loop.Set(i)
	}
	p.seek(p.list()[i].Start)
	p.playing.Set(true)
}

// toggleLoop loops the line playing, or the next one between two lines, or stops looping.
func (p *play) toggleLoop() {
	if p.loop.GetUntracked() >= 0 {
		p.loop.Set(-1)
		return
	}
	i := p.current.Get()
	if i < 0 {
		i = cues.Next(p.list(), p.at.GetUntracked())
	}
	if i >= 0 {
		p.loop.Set(i)
		if p.current.Get() != i {
			p.seek(p.list()[i].Start)
		}
	}
}

// topBar is the film's title with the button back.
func topBar(cx *core.Scope, title string, back func()) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Padding: core.Symmetric(4, 8), MinHeight: core.Pt(44)}, Children: []core.Node{
		ui.IconButton{TestID: "player-back", Icon: lucide.ChevronLeft, LabelFunc: func() string { return locales.PlayerBack(cx) }, Variant: ui.IconButtonGhost, Size: 40, OnPress: back},
		core.Text{Content: title, Role: core.RoleHeader, MaxLines: 1, Style: look.Title.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}), StyleFunc: ui.Colored(colors.Text)},
	}}
}

// clock writes a position as the transport shows it.
func clock(d time.Duration) string { return look.Duration(d) }

// speedText writes a speed as 1×, 0.75×.
func speedText(r float32) string { return fmt.Sprintf("%g×", r) }
