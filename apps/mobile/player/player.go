// Package player plays a film kept on the phone and studies it, in the dark whatever the system's appearance: the video, the card of the line playing, whose words a tap looks up and puts in the vocabulary and which a tap saves, replays, loops or has explained, the transport, and a panel of tabs: the film's lines, which follow the one playing and play from the one tapped, its words looked up or in the vocabulary, the notes on the lines saved, the film in equal parts, and what the film is. Looking a word up, explaining a line and answering questions about it, and the recap of the part watched that a film opened where it was left shows, go through the film's computer, whose AI service answers (session.LookUp, Explain, Ask, Recap); the lines saved, their notes and the words put in the vocabulary sync with it (session.ToggleSave, SetNote, ToggleWord). Where it was left is kept with the film, and so is how far its lines are moved against the picture. The player shows over the app, and minimized it leaves a bar over the tabs while the film plays on (Layer, MiniBar).
//
// The layout follows the window: on a phone held upright the video sits on top, no taller than a share of the screen and as narrow as an upright video's ratio, the card, the transport and the panel under it, all within the screen; turned sideways the video fills the screen with the lines over it, a tap shows the controls over it, and the list of lines can slide in at its side; on a tablet held sideways the panel takes a column beside the video. Upright, the picture's full-screen button lays the player out as turned sideways whatever the window, and turns the screen the way the video is (fullScreen).
package player

import (
	"fmt"
	"path/filepath"
	"time"

	"kando/apps/mobile/film"
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

// layoutOf is the layout of a window w; full screen asked for by the picture's button (forced) is full whatever the window.
func layoutOf(w core.Size, forced bool) layout {
	if forced {
		return full
	}
	switch {
	case w.Width > w.Height && w.Height < 600:
		return full
	case w.Width > w.Height && w.Width >= 900:
		return side
	}
	return upright
}

// The tablet's sizes: the narrowest window that sets the video in from its sides, the panel's column on its side, and the height on its side that the top bar, the card and the transport take under the video.
const (
	tabletWidth    core.Points = 700
	sidePanelWidth core.Points = 380
	sideChrome     core.Points = 400
)

// The upright sizes: the share of the window's height inside its safe area that the picture takes at most, so the card, the transport and the panel keep the rest; the least height the panel keeps under the transport, its tabs and a line of its list, which the card's own scroll gives up room to on a short screen; and the widest window whose full screen is upright, where the controls over the picture stack in two rows and the list of lines does not slide in.
const (
	stageShare             = 0.3
	panelMin   core.Points = 120
	narrowFull core.Points = 520
)

// uprightStage is the picture's size upright, in room, the width the window gives it: at the video's ratio as wide as room, no taller than stageShare of the window's height inside its safe area; a picture so capped, as an upright video's, narrows to its ratio, so no black bars stand beside it.
func uprightStage(room core.Points, win core.Size, safe core.Edges, aspect float32) core.Size {
	a := core.Points(max(aspect, 0.1))
	most := max(win.Height-safe.Top-safe.Bottom, 0) * stageShare
	if room/a <= most {
		return core.Size{Width: room, Height: room / a}
	}
	return core.Size{Width: most * a, Height: most}
}

// view is the player of the film kept under key, over the app while l shows it, else minimized: then its video plays on out of sight, in the mini player's stead, and the rest of it is out of the way.
func view(cx *core.Scope, l *Layer, key string) core.Node {
	theme.Dark(cx)
	s := l.s
	f, _ := s.Film(key)
	// A film of a source matched to a computer's film is named as the computer names it, as the Library lists it (catalog.Merge).
	title := f.LinesFilm().Film.Title
	p := newPlay(cx, s, key)
	l.carry(p)
	shown := l.Full
	r := newRecapper(cx, p, l.held)
	// forced is full screen asked for by the picture's button: the player lays itself out as on a phone on its side whatever the window until its button, Back, minimizing or closing the player ends it.
	forced := core.NewSignal(cx, false)
	shape := core.NewMemo(cx, func() layout { return layoutOf(cx.Env().Window(), forced.Get()) })
	controls := core.NewSignal(cx, true)
	// panelOn shows the list of lines at the side of a phone on its side.
	panelOn := core.NewSignal(cx, false)
	settingsOpen := core.NewSignal(cx, false)
	w := newWords(cx, p)
	fx := newFixer(cx, p)
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
			fx.close()
			settingsOpen.Set(false)
			panelOn.Set(false)
			forced.Set(false)
			p.save()
			return true
		})
		cx.DismissKeyboard()
	})
	// Back, as Android's button or Escape, leaves full screen, else minimizes the player while it shows; the sheets over it take it first.
	cx.OnBackPress(func() bool {
		if !l.full.GetUntracked() {
			return false
		}
		if forced.GetUntracked() {
			forced.Set(false)
			return true
		}
		l.Minimize()
		return true
	})
	// The keyboard's shortcuts act on the player while it shows over the app, not on the film playing on in the mini player.
	keys(cx, p, w, shown)

	// A film kept by its lines alone, as its computer keeps no video of it, shows in the picture's place what can be done to have the video (film.VideoPrompt), and plays it once it is on the phone. A film kept to stream plays from its computer on its network, or from its storage away from it, and shows its lines alone when neither has its video (session.Stream); a film being made plays a link's sound until its video comes, then the video from where it was, as the link changes (session.StreamLink) and the player opens it anew (OnLoad).
	video := func(cx *core.Scope) core.Node {
		return core.Video{TestID: "player-video", Ref: p.ref, Label: title,
			// A film streamed until its download is done plays from the phone from then on, so its file is read as the record has it now.
			SourceFunc: func() string {
				if p.streamed() {
					return s.StreamLink(key)
				}
				if v := s.VideoFile(key); v != "" {
					return v
				}
				k, _ := s.Film(key)
				return filepath.Join(s.Store().FilmDir(k.Desktop, k.Film.ID), k.Video)
			},
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
			OnEnd:      func() { p.ended(l) },
			// A film streamed from its computer that the system's player cannot open from there opens from the storage, where the storage holds it (session.StreamFailed).
			OnError: func(error) {
				if p.streamed() && s.StreamFailed(key) {
					return
				}
				p.failed.Set(true)
			},
			// In picture in picture the film plays on in the system's window over other apps; the player leaves for the tabs once the window shows, and shows again when the window is turned back into it.
			PictureInPicture: true,
			OnPictureInPicture: func(st core.PictureInPictureState) {
				if st.Active && !p.pip.GetUntracked().Active {
					l.Minimize()
				}
				p.pip.Set(st)
			},
			OnPictureInPictureRestore: func() { l.Play(key) },
			OnPress: func(core.PressEvent) {
				if shape.Get() == full {
					controls.Update(func(on bool) bool { return !on })
					return
				}
				p.toggle()
			},
		}
	}
	// Without a video to play, a film kept by its lines and one streamed from a storage that holds only its lines alike say where the video is and how to have it (film.VideoPrompt); a film being made whose sound and video are not in yet says it plays once they are.
	picture := core.Show{When: p.hasVideo, Then: video, Else: func(cx *core.Scope) core.Node {
		if f.Film.Making && s.OnNetwork(f.Desktop) {
			return core.View{Style: fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24), Background: theme.PlayerBG}), Children: []core.Node{
				core.Text{TestID: "player-making", ContentFunc: func() string { d, _ := s.Desktop(f.Desktop); return locales.PlayerMakingWait(cx, d.Name) }, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter, Color: theme.PlayerInk})},
			}}
		}
		return core.View{Style: fill.Merge(core.Style{JustifyContent: core.JustifyCenter, Background: theme.PlayerBG}), Children: []core.Node{
			film.VideoPrompt(cx, s, f.Desktop, f.Film, "player-no-video"),
		}}
	}}
	// A film streamed from a source that cannot be reached says so, at its start or midway, and why (sourceFailed).
	failed := core.Show{When: func() bool { return p.failed.Get() || s.StreamProblem(key) != nil }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24), Gap: 6}), Children: []core.Node{
			core.Text{TestID: "player-failed", ContentFunc: func() string { return failedText(cx, s, key) }, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter, Color: theme.PlayerInk})},
			sourceFailed(cx, s, key),
		}}
	}}
	overlaid := func() bool { return shape.Get() != full || controls.Get() }
	onPicture := func() bool { return shape.Get() == full || p.onPicture.Get() }
	// inset sets the video in from the screen's sides, rounded, as on a tablet: on its side, or upright in a window as wide as one.
	inset := func() bool {
		return shape.Get() == side || (shape.Get() == upright && cx.Env().Window().Width >= tabletWidth)
	}
	// uprightSize is the picture's size upright (uprightStage), in from the sides on a tablet.
	uprightSize := func() core.Size {
		win := cx.Env().Window()
		room := win.Width
		if inset() {
			room -= 48
		}
		return uprightStage(room, win, cx.Env().SafeArea(), p.aspect.Get())
	}
	// narrow is a full screen upright, as a taller than wide video's: the controls over the picture stack, and the list of lines does not slide in.
	narrow := func() bool { return shape.Get() == full && cx.Env().Window().Width < narrowFull }
	panelShown := func() bool { return panelOn.Get() && !narrow() }
	// On its side the phone darkens the picture's top and foot under the controls over it.
	scrim := core.View{PointerEvents: core.PointerEventsNone, Style: fill.Merge(core.Style{Gradients: []core.Gradient{{Kind: core.GradientLinear, Angle: 180, Stops: []core.ColorStop{
		{Color: core.RGBA(0x000000, 0.55), At: core.Pct(0)}, {Color: core.RGBA(0x000000, 0), At: core.Pct(22)},
		{Color: core.RGBA(0x000000, 0), At: core.Pct(55)}, {Color: core.RGBA(0x000000, 0.7), At: core.Pct(100)},
	}}}})}
	stage := core.View{TestID: "player-stage", Style: core.Style{Background: core.RGB(0x000000), Overflow: core.Hidden},
		StyleFunc: func() core.Style {
			if shape.Get() == full {
				return core.Style{FlexGrow: 1, FlexShrink: 1}
			}
			aspect := p.aspect.Get()
			st := core.Style{AlignSelf: core.AlignCenter}
			if shape.Get() == side {
				// On a tablet on its side the video leaves room under it for the card and the transport, which would otherwise fall off a 16:10 screen's foot.
				win := cx.Env().Window()
				width := min(win.Width-sidePanelWidth-48, (win.Height-sideChrome)*core.Points(aspect))
				st.Width, st.AspectRatio = core.Pt(max(width, 0)), aspect
			} else {
				size := uprightSize()
				st.Width, st.Height = core.Pt(size.Width), core.Pt(size.Height)
			}
			if inset() {
				st.Margin, st.CornerRadius, st.BorderCurve = core.Edges{Left: 24, Right: 24}, 18, core.BorderCurveContinuous
				st.Shadows = []core.BoxShadow{{Offset: core.Point{Y: 20}, Blur: 50, Color: core.RGBA(0x000000, 0.4)}}
			}
			return st
		},
		Children: []core.Node{picture, soundPoster(cx, s, key, f.Film.Sound, p), audioOnly(cx, p),
			core.Show{When: func() bool { return shape.Get() == full && overlaid() }, Then: func(*core.Scope) core.Node { return scrim }},
			nextCard(cx, l, p),
			core.Show{When: onPicture, Then: func(cx *core.Scope) core.Node { return subtitles(cx, p, w, shape.Get, overlaid, uprightSize, narrow) }},
			pausedMark(cx, p, func() bool { return shape.Get() == full }),
			pills(cx, p, overlaid, func() bool { return shape.Get() == full }, narrow), failed,
			// Upright, a button at the picture's corner shows it on the whole screen.
			core.Show{When: func() bool { return shape.Get() == upright && p.hasVideo() && !p.failed.Get() }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}, Children: []core.Node{
					roundButton("player-fullscreen", lucide.Maximize, 36, 18, func() string { return locales.PlayerFullScreen(cx) }, func() core.Color { return core.RGBA(0x000000, 0.45) }, nil, func() {
						controls.Set(true)
						forced.Set(true)
					}),
				}}
			}}}}

	notFull := func() bool { return shape.Get() != full }
	bar := core.Show{When: notFull,
		Then: func(cx *core.Scope) core.Node {
			return topBar(cx, title, l.Minimize, func() { settingsOpen.Set(true) })
		},
		Else: func(cx *core.Scope) core.Node {
			return landscapeTop(cx, p, title, l.Minimize, func() { settingsOpen.Set(true) }, panelOn, forced, narrow)
		}}
	controlsNode := core.Show{When: notFull,
		Then: func(cx *core.Scope) core.Node { return transport(cx, p) },
		Else: func(cx *core.Scope) core.Node { return capsule(cx, p, narrow) }}
	// On its side the phone shows its top row and its capsule of controls over the video while the controls show; minimized, neither shows.
	over := func(top bool) func() core.Style {
		return func() core.Style {
			if !shown() {
				return core.Style{Display: core.DisplayNone}
			}
			if shape.Get() != full {
				return core.Style{}
			}
			st := core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), ZIndex: 2,
				PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea, Right: core.EnvSafeArea}}
			if top {
				st.Top, st.Padding = core.Pt(0), core.Edges{Top: 12, Right: 16, Left: 12}
				st.PaddingEnv.Top = core.EnvSafeArea
			} else {
				st.Bottom, st.Padding = core.Pt(0), core.Edges{Right: 16, Bottom: 12, Left: 16}
				st.PaddingEnv.Bottom = core.EnvSafeArea
			}
			if !controls.Get() {
				st.Display = core.DisplayNone
			}
			return st
		}
	}
	// The card scrolls in itself on a screen too short for it over the panel's least height, so the transport under it stays on the screen.
	cardPart := core.Scroll{TestID: "player-card-scroll", HideIndicator: true, Style: core.Style{MinHeight: core.Pt(0)}, StyleFunc: func() core.Style {
		if shape.Get() == full || !shown() {
			return core.Style{Display: core.DisplayNone}
		}
		return core.Style{}
	}, Children: []core.Node{core.View{StyleFunc: func() core.Style {
		if inset() {
			return core.Style{Padding: core.Edges{Top: 16, Right: 24, Left: 24}}
		}
		return core.Style{Padding: core.Edges{Top: 14, Right: 14, Left: 14}}
	}, Children: []core.Node{card(cx, p, w)}}}}
	main := core.View{TestID: "player-main", StyleFunc: func() core.Style {
		st := core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), MinHeight: core.Pt(0)}
		switch {
		case shape.Get() == upright:
			// Upright the panel takes what is left under the transport; on a short screen it keeps its least height and the card's scroll gives way.
			st.FlexGrow = 0
		case shape.Get() == full && panelShown() && shown():
			// With the list of lines slid in, the picture and the controls over it keep to its left.
			st.Margin = core.Edges{Right: 300}
		}
		return st
	}, Children: []core.Node{
		core.View{StyleFunc: over(true), Children: []core.Node{bar}},
		stage,
		cardPart,
		core.View{StyleFunc: func() core.Style {
			if shape.Get() != full && inset() {
				return over(false)().Merge(core.Style{Padding: core.Edges{Right: 12, Left: 12}})
			}
			return over(false)()
		}, Children: []core.Node{controlsNode}},
	}}
	tabs := panel(cx, p, w)
	list := core.View{TestID: "player-lines-pane", StyleFunc: func() core.Style {
		if !shown() {
			return core.Style{Display: core.DisplayNone}
		}
		switch shape.Get() {
		case full:
			if !panelShown() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Width: core.Pt(300), ZIndex: 3, Padding: core.Edges{Top: 8, Bottom: 8},
				Background: theme.PlayerSheet.Faded(0.96), Borders: core.Edges{Left: 0.5}, BorderColors: core.EdgeColors{Left: theme.PlayerInk.Faded(0.12)},
				PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Bottom: core.EnvSafeArea}}
		case side:
			return core.Style{Width: core.Pt(sidePanelWidth), PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea},
				Background: theme.PlayerInk.Faded(0.04), Borders: core.Edges{Left: 0.5}, BorderColors: core.EdgeColors{Left: theme.PlayerInk.Faded(0.12)}}
		}
		return core.Style{FlexGrow: 1, FlexShrink: 0, FlexBasis: core.Pt(0), MinHeight: core.Pt(panelMin), PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}
	}, Children: []core.Node{
		// On its side a phone's panel is the lines alone.
		core.Show{When: func() bool { return shape.Get() == full }, Then: func(cx *core.Scope) core.Node { return lines(cx, p, true) }, Else: func(*core.Scope) core.Node { return tabs }},
	}}

	return core.View{TestID: "player", PointerEvents: core.PointerEventsAuto, AccessibilityElementsHidden: func() bool { return !shown() },
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), ZIndex: 20},
		StyleFunc: func() core.Style {
			if !shown() {
				// Out of sight, the video plays on, at a size of its own so the platform keeps it.
				return core.Style{Width: core.Pt(2), Height: core.Pt(2), Overflow: core.Hidden, Opacity: core.Alpha(0), PointerEvents: core.PointerEventsNone}
			}
			st := core.Style{Right: core.Pt(0), Bottom: core.Pt(0), Background: theme.PlayerBG}
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
		Children: []core.Node{backdrop(cx, p, func() bool { return shown() && shape.Get() != full }), main, list,
			wordPopover(cx, p, w, func() bool { return shape.Get() == full }),
			wordSheet(cx, p, w, func() bool { return shape.Get() != full }),
			explainSheet(cx, p, w, fx),
			fixSheet(cx, p, fx),
			recapSheet(cx, p, r),
			settingsSheet(cx, p, l, settingsOpen),
			fullScreen(forced, p, true), fullScreen(forced, p, false),
			core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
				// At the top, under the player's top bar, clear of the lines and the transport.
				return ui.Toasts{Toaster: p.toast, Placement: ui.ToastTop, Inset: 46, TestID: "player-toasts"}
			}},
		}}
}

// fullScreen asks the system, while full screen is on (forced), to show the app sideways for a video wider than tall, wide, else upright for one taller than wide, whose full screen stays upright. Each request lives with its own scope, so leaving full screen, minimizing or closing the player gives the orientation back to the system, and turning the phone back upright keeps a wide video sideways until full screen ends. A video whose ratio changes as it loads moves from one request to the other.
func fullScreen(forced *core.Signal[bool], p *play, wide bool) core.Node {
	mask := core.OrientationPortrait
	if wide {
		mask = core.OrientationLandscape
	}
	return core.Show{When: func() bool { return forced.Get() && (p.aspect.Get() >= 1) == wide }, Then: func(cx *core.Scope) core.Node {
		cx.RequestOrientation(mask)
		return core.Group(nil)
	}}
}

// fill fills the parent, over what is in it.
var fill = core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}

// topBar is the button that minimizes the player, the film's title in the middle, and the button of the playback settings.
// The design's chapter line over the title is left out: the phone has no chapters of a film (sync.Film has none).
func topBar(cx *core.Scope, title string, minimize, settings func()) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 10, Right: 6, Bottom: 6, Left: 6}, MinHeight: core.Pt(60)}, Children: []core.Node{
		ui.IconButton{TestID: "player-minimize", Icon: lucide.ChevronDown, LabelFunc: func() string { return locales.PlayerMinimize(cx) }, Variant: ui.IconButtonGhost, Size: 44, IconSize: 26, OnPress: minimize},
		core.Text{Content: title, Role: core.RoleHeader, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, TextAlign: core.TextAlignCenter, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
			StyleFunc: ui.Colored(colors.Text)},
		ui.IconButton{TestID: "player-more", Icon: lucide.Ellipsis, LabelFunc: func() string { return locales.PlayerSettings(cx) }, Variant: ui.IconButtonGhost, Size: 44, IconSize: 26, OnPress: settings},
	}}
}

// overPicture is the shadow under text drawn on the picture, which keeps it legible on a light one.
var overPicture = core.Style{TextShadowColor: core.RGBA(0x000000, 0.9), TextShadowRadius: 3, Color: theme.PlayerInk}

// roundButton is a round button over the picture, its icon white on fill.
func roundButton(id string, icon func() *core.SVG, size, glyph core.Points, label func() string, fill func() core.Color, selected func() bool, press func()) core.Node {
	return base.Button{TestID: id, LabelFunc: label, Selected: selected, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			BackdropFilters: []core.Filter{core.Blur(10)}, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return core.Style{Background: fill()} },
		Children:  []core.Node{ui.IconOf(icon, glyph, func() core.Color { return theme.PlayerInk })}}
}

// landscapeTop is a phone on its side's row over the picture: the button that minimizes the player and the film's title at the left; at the right the subtitle mode, a tap on which goes on to the next, the button that slides the list of lines in, except on an upright full screen (narrow), the button that leaves full screen while it is on (forced), and the playback settings.
func landscapeTop(cx *core.Scope, p *play, title string, minimize, settings func(), panelOn, forced *core.Signal[bool], narrow func() bool) core.Node {
	glass := func() core.Color { return theme.PlayerInk.Faded(0.14) }
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
		ui.IconButton{TestID: "player-minimize", Icon: lucide.ChevronLeft, LabelFunc: func() string { return locales.PlayerMinimize(cx) }, Variant: ui.IconButtonGhost, Size: 40, IconSize: 26,
			Tint: func() core.Color { return theme.PlayerInk }, OnPress: minimize},
		core.Text{Content: title, Role: core.RoleHeader, MaxLines: 1, Style: overPicture.Merge(core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold, FlexShrink: 1, MinWidth: core.Pt(0)})},
		core.View{Style: core.Style{FlexGrow: 1}},
		base.Button{TestID: "player-mode-pill", LabelFunc: func() string { return locales.PlayerModePill(cx, modeShort(cx, p.mode.Get())) }, OnPress: func(core.PressEvent) { p.nextMode() },
			Style: core.Style{Height: core.Pt(32), Padding: core.Symmetric(0, 14), CornerRadius: 16, JustifyContent: core.JustifyCenter, Background: theme.PlayerInk.Faded(0.16),
				BackdropFilters: []core.Filter{core.Blur(10)}, Active: &core.Style{Opacity: core.Alpha(0.7)}},
			Children: []core.Node{core.Text{ContentFunc: func() string { return locales.PlayerModePill(cx, modeShort(cx, p.mode.Get())) }, MaxLines: 1,
				Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightBold, Color: theme.PlayerInk}}}},
		core.Show{When: func() bool { return !narrow() }, Then: func(cx *core.Scope) core.Node {
			return roundButton("player-panel-toggle", lucide.PanelRight, 40, 20, func() string { return locales.PlayerLines(cx) }, func() core.Color {
				if panelOn.Get() {
					return theme.PlayerInk.Faded(0.28)
				}
				return glass()
			}, panelOn.Get, func() { panelOn.Update(func(on bool) bool { return !on }) })
		}},
		core.Show{When: forced.Get, Then: func(cx *core.Scope) core.Node {
			return roundButton("player-fullscreen-exit", lucide.Minimize, 40, 20, func() string { return locales.PlayerExitFullScreen(cx) }, glass, nil, func() { forced.Set(false) })
		}},
		roundButton("player-more", lucide.Ellipsis, 40, 20, func() string { return locales.PlayerSettings(cx) }, glass, nil, settings),
	}}
}

// backdrop is the film's poster blurred behind the player's top, fading into its dark.
func backdrop(cx *core.Scope, p *play, when func() bool) core.Node {
	return core.View{PointerEvents: core.PointerEventsNone, AccessibilityElementsHidden: func() bool { return true },
		Style: fill.Merge(core.Style{Overflow: core.Hidden}),
		StyleFunc: func() core.Style {
			if !when() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		},
		Children: []core.Node{
			core.Image{ResizeMode: core.Cover, ElementsHidden: true,
				Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Height: core.Pct(70),
					Filters: []core.Filter{core.Blur(50), core.Saturate(1.4)}, Opacity: core.Alpha(0.55)},
				SourceFunc: func() core.ImageSource {
					f, _ := p.s.Film(p.key)
					return look.LocalPoster(p.s, f)
				}},
			core.View{Style: fill.Merge(core.Style{Gradients: []core.Gradient{{Kind: core.GradientLinear, Angle: 180, Stops: []core.ColorStop{
				{Color: theme.PlayerBG.Faded(0.2), At: core.Pct(0)}, {Color: theme.PlayerBG.Faded(0.85), At: core.Pct(45)}, {Color: theme.PlayerBG, At: core.Pct(70)},
			}}}})},
		}}
}

// pausedMark is the play mark in the middle of the picture while the film holds.
func pausedMark(cx *core.Scope, p *play, wide func() bool) core.Node {
	return core.Show{When: func() bool { return !p.playing.Get() && p.hasVideo() && !p.failed.Get() }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "player-paused", PointerEvents: core.PointerEventsNone, Style: fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}),
			StyleFunc: func() core.Style {
				if wide() {
					// On its side the mark sits a little above the middle, clear of the lines and the controls under it.
					return core.Style{Padding: core.Edges{Bottom: 64}}
				}
				return core.Style{}
			},
			Children: []core.Node{core.View{
				Style: core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: theme.PlayerInk.Faded(0.22), BackdropFilters: []core.Filter{core.Blur(12)}},
				StyleFunc: func() core.Style {
					size := core.Points(62)
					if wide() {
						size = 66
					}
					return core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2}
				},
				Children: []core.Node{core.View{Style: core.Style{Padding: core.Edges{Left: 3}}, Children: []core.Node{ui.IconOf(playGlyph, 26, func() core.Color { return theme.PlayerInk })}}},
			}}}
	}}
}

// soundPoster covers the black picture of a film of sound alone (sync.Film.Sound), as a podcast's, with its poster, its own art or a picture of its sound as the computer drew it, as a music player shows an album's.
func soundPoster(cx *core.Scope, s *session.Session, key string, sound bool, p *play) core.Node {
	return core.Show{When: func() bool { return sound && p.hasVideo() }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "player-sound", PointerEvents: core.PointerEventsNone, Style: fill.Merge(core.Style{Background: theme.PlayerBG, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}),
			Children: []core.Node{
				ui.IconOf(lucide.AudioLines, 36, func() core.Color { return theme.PlayerInk.Faded(0.35) }),
				core.Image{TestID: "player-sound-poster", ResizeMode: core.Contain, Style: fill, SourceFunc: func() core.ImageSource {
					k, _ := s.Film(key)
					return look.LocalPoster(s, k)
				}},
			}}
	}}
}

// audioOnly covers the picture while audio only is on: three bars that beat as the sound plays, and that the film plays on with the screen locked.
func audioOnly(cx *core.Scope, p *play) core.Node {
	return core.Show{When: p.audioOnly.Get, Then: func(cx *core.Scope) core.Node {
		bar := func(height core.Points, delay time.Duration) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(3), Height: core.Pt(height), CornerRadius: 1.5, Background: theme.PlayerInk,
				Animation: core.KeyframeAnimation{Keyframes: core.KeyframesPulse, Duration: 900 * time.Millisecond, Delay: delay}}}
		}
		white := theme.PlayerInk
		return core.View{TestID: "player-audio-only", PointerEvents: core.PointerEventsNone,
			Style: fill.Merge(core.Style{Background: theme.PlayerBG, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6}),
			Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignEnd, Gap: 3, Height: core.Pt(28), Margin: core.Edges{Bottom: 4}}, Children: []core.Node{
					bar(16, 0), bar(28, 300*time.Millisecond), bar(20, 600*time.Millisecond),
				}},
				core.Text{ContentFunc: func() string { return locales.PlayerAudioOnly(cx) }, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, Color: white}},
				core.Text{TestID: "player-audio-only-note", ContentFunc: func() string { return locales.PlayerAudioOnlyCover(cx) }, Style: core.Style{FontSize: 12, LineHeight: 16, Color: white.Faded(0.6)}},
			}}
	}}
}

// pills are the states of the player over the picture's corner: the line looped, shadowing, a speed other than 1×, the sleep timer and the lines moved, clear of the full-screen button upright; on a phone on its side they sit under the row over the picture, and under the status bar too on an upright full screen (narrow).
func pills(cx *core.Scope, p *play, shown, wide, narrow func() bool) core.Node {
	dark := func() core.Color { return core.RGBA(0x000000, 0.55) }
	pill := func(id string, when func() bool, text func() string, fill func() core.Color) core.Node {
		return core.Show{When: when, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: id, Style: core.Style{Height: core.Pt(24), Padding: core.Symmetric(0, 10), CornerRadius: 12, JustifyContent: core.JustifyCenter},
				StyleFunc: func() core.Style { return core.Style{Background: fill()} },
				Children: []core.Node{
					core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold, Color: theme.PlayerInk}},
				}}
		}}
	}
	return core.View{TestID: "player-pills", PointerEvents: core.PointerEventsNone,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(10), Right: core.Pt(10), Direction: core.Row, Wrap: core.Wrap, Gap: 6},
		StyleFunc: func() core.Style {
			if !shown() {
				return core.Style{Display: core.DisplayNone}
			}
			if wide() {
				st := core.Style{Top: core.Pt(64), PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea}}
				if narrow() {
					st.PaddingEnv.Top = core.EnvSafeArea
				}
				return st
			}
			return core.Style{Top: core.Pt(10), Right: core.Pt(52)}
		},
		Children: []core.Node{
			pill("player-pill-loop", func() bool { return p.loop.Get() >= 0 }, func() string { return locales.PlayerLoopLine(cx) }, func() core.Color { return theme.PlayerBlue }),
			pill("player-pill-follow", p.follow.Get, func() string {
				if p.held.Get() {
					return locales.PlayerFollowHeld(cx)
				}
				return locales.PlayerFollow(cx)
			}, func() core.Color {
				if p.held.Get() {
					return theme.StatusWarn
				}
				return dark()
			}),
			pill("player-pill-speed", func() bool { return p.speed() != 1 }, func() string { return speedText(p.speed()) }, dark),
			pill("player-pill-sleep", func() bool { return p.sleep.Get() != sleepOff }, func() string { return sleepPill(cx, p) }, dark),
			pill("player-pill-delay", func() bool { return p.delay.Get() != 0 }, func() string { return locales.PlayerDelayPill(cx, delayText(p.delay.Get())) }, dark),
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

// speedFace writes the speed on the transport's pill, a whole one with its tenth: 1.0×, 0.75×.
func speedFace(r float32) string {
	if r == float32(int(r)) {
		return fmt.Sprintf("%.1f×", r)
	}
	return speedText(r)
}

// delayText writes how far the lines are moved as +0.3, -1.2, 0.0.
func delayText(d time.Duration) string {
	s := fmt.Sprintf("%.1f", d.Seconds())
	if d > 0 {
		s = "+" + s
	}
	return s
}
