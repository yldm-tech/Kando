// Package player plays a film kept on the phone and studies it, in the dark whatever the system's appearance: the video, the card of the line playing, whose words a tap looks up and puts in the vocabulary and which a tap saves, replays, loops or has explained, the transport, and a panel of tabs: the film's lines, which follow the one playing and play from the one tapped, its words looked up or in the vocabulary, the notes on the lines saved, the film in equal parts, and what the film is. Looking a word up, explaining a line and answering questions about it, and the recap of the part watched that a film opened where it was left shows, go through the film's computer, whose AI service answers (session.LookUp, Explain, Ask, Recap); the lines saved, their notes and the words put in the vocabulary sync with it (session.ToggleSave, SetNote, ToggleWord). Where it was left is kept with the film, and so is how far its lines are moved against the picture. The player shows over the app, and minimized it leaves a bar over the tabs while the film plays on (Layer, MiniBar).
//
// The layout follows the window: on a phone held upright the video sits on top, the card, the transport and the panel under it; turned sideways the video fills the screen with the lines over it, a tap shows the controls over it, and the list of lines can slide in at its side; on a tablet held sideways the panel takes a column beside the video.
package player

import (
	"fmt"
	"path/filepath"
	"time"

	"kando/apps/mobile/film"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Speeds are the speeds the player offers.
var Speeds = []float32{0.5, 0.75, 1, 1.25, 1.5}

// Modes are the subtitle modes the player offers, in the order its menu lists them: both lines, the original alone, the translation alone, and neither, where the card blurs both until a tap shows them.
var Modes = []string{kcore.SubBilingual, kcore.SubOriginal, kcore.SubTranslation, kcore.SubOff}

// The settings the player keeps in the app's storage, for every film: the subtitle mode, the speed, shadowing, audio only and the lines on the picture while upright.
const (
	modeKey      = "player.mode"
	speedKey     = "player.speed"
	followKey    = "player.follow"
	audioOnlyKey = "player.audioOnly"
	onPictureKey = "player.onPicture"
)

// saveEvery is how often, in the film's time, the position playing is kept.
const saveEvery = 5 * time.Second

// layout is how the player lays itself out in the window.
type layout uint8

const (
	// upright is a phone held upright, or a narrow window: video, card, transport, panel.
	upright layout = iota
	// full is a phone on its side: the video fills the screen, the controls over it.
	full
	// side is a tablet on its side: the video, card and transport, the panel beside them.
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

// view is the player of the film kept under key, over the app while l shows it, else minimized: then its video plays on out of sight, in the mini player's stead, and the rest of it is out of the way.
func view(cx *core.Scope, l *Layer, key string) core.Node {
	theme.Dark(cx)
	s := l.s
	f, _ := s.Film(key)
	p := newPlay(cx, s, key)
	shown := l.Full
	r := newRecapper(cx, p, l.held)
	shape := core.NewMemo(cx, func() layout { return layoutOf(cx.Env().Window()) })
	controls := core.NewSignal(cx, true)
	// panelOn shows the list of lines at the side of a phone on its side.
	panelOn := core.NewSignal(cx, false)
	settingsOpen := core.NewSignal(cx, false)
	w := newWords(cx, p)
	p.status = func() core.StatusBarSetting {
		hidden := core.ToggleOff
		if shape.Get() == full && !controls.Get() {
			hidden = core.ToggleOn
		}
		return core.StatusBarSetting{Foreground: core.StatusBarLight, Hidden: hidden}
	}
	l.cur.Set(p)
	cx.Cleanup(func() {
		if l.cur.GetUntracked() == p {
			l.cur.Set(nil)
		}
	})
	// Minimized, the player keeps where the film is, and what showed over it closes, the keyboard with it.
	cx.Effect(func() {
		if shown() {
			return
		}
		core.Untracked(cx, func() bool {
			w.closeLookup()
			w.closeExplain()
			settingsOpen.Set(false)
			panelOn.Set(false)
			p.save()
			return true
		})
		cx.DismissKeyboard()
	})
	// Back, as Android's button or Escape, minimizes the player while it shows; the sheets over it take it first.
	cx.OnBackPress(func() bool {
		if !l.full.GetUntracked() {
			return false
		}
		l.Minimize()
		return true
	})
	// The keyboard's shortcuts act on the player while it shows over the app, not on the film playing on in the mini player.
	keys(cx, p, w, shown)

	// A film kept by its lines alone, as its computer keeps no video of it, shows in the picture's place what can be done to have the video (film.VideoPrompt), and plays it once it is on the phone. A film kept to stream plays from its computer's storage, and shows its lines alone when the storage holds no video of it (session.Stream).
	video := func(cx *core.Scope) core.Node {
		k, _ := s.Film(key)
		source := filepath.Join(s.Store().FilmDir(k.Desktop, k.Film.ID), k.Video)
		if p.streamed() {
			source = s.StreamLink(key)
		}
		return core.Video{TestID: "player-video", Ref: p.ref, Label: f.Film.Title,
			Source:           source,
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
				// Where the film was left, or where its lines were followed to while its video was not on the phone yet.
				if at := p.at.GetUntracked(); at > 0 && (info.Duration == 0 || at < info.Duration-2*time.Second) {
					p.seek(at)
				}
			},
			OnProgress: p.progress,
			OnEnd:      p.ended,
			OnError:    func(error) { p.failed.Set(true) },
			OnPress: func(core.PressEvent) {
				if shape.Get() == full {
					controls.Update(func(on bool) bool { return !on })
					return
				}
				p.toggle()
			},
		}
	}
	// Without a video to play, a film kept by its lines and one streamed from a storage that holds only its lines alike say where the video is and how to have it (film.VideoPrompt).
	picture := core.Show{When: p.hasVideo, Then: video, Else: func(cx *core.Scope) core.Node {
		return core.View{Style: fill.Merge(core.Style{JustifyContent: core.JustifyCenter, Background: core.RGB(0x0B0B0D)}), Children: []core.Node{
			film.VideoPrompt(cx, s, f.Desktop, f.Film, "player-no-video"),
		}}
	}}
	failed := core.Show{When: p.failed.Get, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24)}), Children: []core.Node{
			core.Text{TestID: "player-failed", ContentFunc: func() string { return locales.PlayerCantPlay(cx) }, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter, Color: core.RGB(0xFFFFFF)})},
		}}
	}}
	overlaid := func() bool { return shape.Get() != full || controls.Get() }
	onPicture := func() bool { return shape.Get() == full || p.onPicture.Get() }
	stage := core.View{TestID: "player-stage", Style: core.Style{Background: core.RGB(0x000000), Overflow: core.Hidden},
		StyleFunc: func() core.Style {
			if shape.Get() == full {
				return core.Style{FlexGrow: 1, FlexShrink: 1}
			}
			return core.Style{Width: core.Pct(100), AspectRatio: max(p.aspect.Get(), 1)}
		},
		Children: []core.Node{picture, audioOnly(cx, p),
			core.Show{When: onPicture, Then: func(cx *core.Scope) core.Node { return subtitles(cx, p, w, shape.Get, overlaid) }},
			pills(cx, p, func() bool { return shape.Get() != full || controls.Get() }), failed}}

	bar := topBar(cx, p, f.Film.Title, l.Minimize, func() { settingsOpen.Set(true) }, shape.Get, panelOn)
	transport := transport(cx, p)
	// On its side the phone shows the top bar and the transport over the video while the controls show; minimized, neither shows.
	over := func(top bool) func() core.Style {
		return func() core.Style {
			if !shown() {
				return core.Style{Display: core.DisplayNone}
			}
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
	cardPart := core.View{StyleFunc: func() core.Style {
		if shape.Get() == full || !shown() {
			return core.Style{Display: core.DisplayNone}
		}
		return core.Style{Padding: core.Edges{Top: 10, Right: 12, Left: 12}}
	}, Children: []core.Node{card(cx, p, w)}}
	main := core.View{TestID: "player-main", StyleFunc: func() core.Style {
		st := core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}
		if shape.Get() == upright {
			st.FlexGrow, st.FlexShrink = 0, 0
		}
		return st
	}, Children: []core.Node{
		core.View{StyleFunc: over(true), Children: []core.Node{bar}},
		stage,
		cardPart,
		core.View{StyleFunc: over(false), Children: []core.Node{transport}},
	}}
	tabs := panel(cx, p, w)
	list := core.View{TestID: "player-lines-pane", StyleFunc: func() core.Style {
		if !shown() {
			return core.Style{Display: core.DisplayNone}
		}
		switch shape.Get() {
		case full:
			if !panelOn.Get() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Width: core.Pt(300), ZIndex: 3,
				Background: core.RGBA(0x000000, 0.78), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Bottom: core.EnvSafeArea}}
		case side:
			return core.Style{Width: core.Pt(380), PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea}}
		}
		return core.Style{FlexGrow: 1, FlexShrink: 1, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}
	}, Children: []core.Node{
		// On its side a phone's panel is the lines alone.
		core.Show{When: func() bool { return shape.Get() == full }, Then: func(cx *core.Scope) core.Node { return lines(cx, p) }, Else: func(*core.Scope) core.Node { return tabs }},
	}}

	colors := ui.ColorsOf(cx)
	return core.View{TestID: "player", PointerEvents: core.PointerEventsAuto, AccessibilityElementsHidden: func() bool { return !shown() },
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), ZIndex: 20},
		StyleFunc: func() core.Style {
			if !shown() {
				// Out of sight, the video plays on, at a size of its own so the platform keeps it.
				return core.Style{Width: core.Pt(2), Height: core.Pt(2), Overflow: core.Hidden, Opacity: core.Alpha(0), PointerEvents: core.PointerEventsNone}
			}
			st := core.Style{Right: core.Pt(0), Bottom: core.Pt(0), Background: colors.Background()}
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
		Children: []core.Node{main, list,
			wordPopover(cx, p, w, func() bool { return shape.Get() == full }),
			wordSheet(cx, p, w, func() bool { return shape.Get() != full }),
			explainSheet(cx, p, w),
			recapSheet(cx, p, r),
			settingsSheet(cx, p, settingsOpen),
			core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
				return ui.Toasts{Toaster: p.toast, Placement: ui.ToastBottom, TestID: "player-toasts"}
			}},
		}}
}

// fill fills the parent, over what is in it.
var fill = core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}

// topBar is the button that minimizes the player, the film's title, and the button of the playback settings; on a phone on its side, the button that slides the list of lines in too.
func topBar(cx *core.Scope, p *play, title string, minimize, settings func(), shape func() layout, panelOn *core.Signal[bool]) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Padding: core.Symmetric(4, 8), MinHeight: core.Pt(44)}, Children: []core.Node{
		ui.IconButton{TestID: "player-minimize", Icon: lucide.ChevronDown, LabelFunc: func() string { return locales.PlayerMinimize(cx) }, Variant: ui.IconButtonGhost, Size: 40, OnPress: minimize},
		core.Text{Content: title, Role: core.RoleHeader, MaxLines: 1, Style: look.Title.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}), StyleFunc: ui.Colored(colors.Text)},
		core.Show{When: func() bool { return shape() == full }, Then: func(cx *core.Scope) core.Node {
			return ui.IconButton{TestID: "player-panel-toggle", Icon: lucide.PanelRight, LabelFunc: func() string { return locales.PlayerLines(cx) }, Variant: ui.IconButtonGhost, Size: 40,
				Selected: panelOn.Get, OnPress: func() { panelOn.Update(func(on bool) bool { return !on }) }}
		}},
		ui.IconButton{TestID: "player-more", Icon: lucide.Ellipsis, LabelFunc: func() string { return locales.PlayerSettings(cx) }, Variant: ui.IconButtonGhost, Size: 40, OnPress: settings},
	}}
}

// audioOnly covers the picture while audio only is on.
func audioOnly(cx *core.Scope, p *play) core.Node {
	return core.Show{When: p.audioOnly.Get, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "player-audio-only", PointerEvents: core.PointerEventsNone,
			Style: fill.Merge(core.Style{Background: core.RGB(0x0B0B0D), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 8}),
			Children: []core.Node{
				ui.IconOf(lucide.Headphones, 36, func() core.Color { return core.RGB(0xFFFFFF) }),
				core.Text{ContentFunc: func() string { return locales.PlayerAudioOnly(cx) }, Style: look.Title.Merge(core.Style{Color: core.RGB(0xFFFFFF)})},
			}}
	}}
}

// pills are the states of the player over the picture's corner: the line looped, shadowing, a speed other than 1×, the sleep timer and the lines moved.
func pills(cx *core.Scope, p *play, shown func() bool) core.Node {
	pill := func(id string, when func() bool, text func() string, tone func() core.Color) core.Node {
		return core.Show{When: when, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: id, Style: core.Style{Padding: core.Symmetric(3, 9), CornerRadius: 11, Background: core.RGBA(0x000000, 0.6)}, Children: []core.Node{
				core.Text{ContentFunc: text, MaxLines: 1, Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: func() core.Style { return core.Style{Color: tone()} }},
			}}
		}}
	}
	white := func() core.Color { return core.RGB(0xFFFFFF) }
	return core.View{TestID: "player-pills", PointerEvents: core.PointerEventsNone,
		Style: core.Style{Position: core.Absolute, Top: core.Pt(10), Left: core.Pt(10), Right: core.Pt(10), Direction: core.Row, Wrap: core.Wrap, Gap: 6},
		StyleFunc: func() core.Style {
			if !shown() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		},
		Children: []core.Node{
			pill("player-pill-loop", func() bool { return p.loop.Get() >= 0 }, func() string { return locales.PlayerLoopLine(cx) }, func() core.Color { return theme.SubtitleYellow }),
			pill("player-pill-follow", p.follow.Get, func() string {
				if p.held.Get() {
					return locales.PlayerFollowHeld(cx)
				}
				return locales.PlayerFollow(cx)
			}, func() core.Color { return theme.StatusWarn }),
			pill("player-pill-speed", func() bool { return p.speed() != 1 }, func() string { return speedText(p.speed()) }, white),
			pill("player-pill-sleep", func() bool { return p.sleep.Get() != sleepOff }, func() string { return sleepPill(cx, p) }, white),
			pill("player-pill-delay", func() bool { return p.delay.Get() != 0 }, func() string { return locales.PlayerDelayPill(cx, delayText(p.delay.Get())) }, white),
		}}
}

// sleepPill is what the sleep timer's pill says: how long until it holds the film, or that it holds it after the line or the film.
func sleepPill(cx *core.Scope, p *play) string {
	switch m := p.sleep.Get(); m {
	case sleepLine:
		return locales.PlayerSleepPillLine(cx)
	case sleepFilm:
		return locales.PlayerSleepPillFilm(cx)
	default:
		// Read as the film plays, so the time left counts down.
		_ = p.at.Get()
		return locales.PlayerSleepPillIn(cx, clock(max(p.sleepUntil.Sub(p.now()), 0)))
	}
}

// clock writes a position as the transport shows it.
func clock(d time.Duration) string { return look.Duration(d) }

// speedText writes a speed as 1×, 0.75×.
func speedText(r float32) string { return fmt.Sprintf("%g×", r) }

// delayText writes how far the lines are moved as +0.3, -1.2, 0.0.
func delayText(d time.Duration) string {
	s := fmt.Sprintf("%.1f", d.Seconds())
	if d > 0 {
		s = "+" + s
	}
	return s
}
