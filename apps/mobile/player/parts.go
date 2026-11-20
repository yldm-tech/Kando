package player

import (
	"slices"
	"time"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/substyle"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The faces of the lines over the video, as the design draws them: the translation bold in white over the original in yellow, with no box, a shadow under each keeping it legible, in the design's sizes by layout (substyle.OnPicture); Settings › Subtitle Style changes the original's color, their size and where they show (package substyle).
var (
	translationColor = theme.PlayerInk
	originalColor    = theme.SubtitleYellow
)

// mono is the face of the player's times and places, the system's fixed-pitch one.
var mono = core.Style{FontFamily: core.FontMonospace}

// subtitles draws the line playing over the video, as the mode picks its texts, up out of the controls' way while they show over the video; a tap on a word of its original looks the word up, and the original's words show their marks (markSpans).
func subtitles(cx *core.Scope, p *play, w *words, shape func() layout, controlsOver func() bool) core.Node {
	shown := core.NewMemoFunc(cx, func() []kcore.CueLine {
		i, list := p.current.Get(), p.list()
		if i < 0 || i >= len(list) {
			return nil
		}
		return kcore.PictureLines(p.shownMode(), kcore.SubTranslationFirst, kcore.UntranslatedDim, list[i].Translation, list[i].Original)
	}, slices.Equal[[]kcore.CueLine])
	tone := toneOf(ui.ColorsOf(cx))
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		style := p.s.Prefs().Subtitles
		wide := shape() != upright
		tr, orig := substyle.OnPicture(style, wide)
		height := float32(1.25)
		if wide {
			height = 1.3
		}
		var children []core.Node
		i := p.current.GetUntracked()
		for _, l := range shown.Get() {
			size, color, weight := tr, translationColor, core.FontWeightBold
			if l.Original {
				size, color, weight = orig, substyle.Color(style.Color), core.FontWeightMedium
			}
			st := overPicture.Merge(core.Style{FontSize: size, LineHeightMultiple: height, FontWeight: weight, Color: color, TextAlign: core.TextAlignCenter, MaxWidth: core.Pct(100)})
			if l.Dim {
				st.Opacity = core.Alpha(0.6)
			}
			text := core.Text{Content: l.Text, Style: st}
			if l.Original && !l.Dim {
				text = w.tappable(text, i, l.Text, tone)
			}
			children = append(children, text)
		}
		return core.Group(children)
	})
	return core.View{TestID: "player-subtitles", PointerEvents: core.PointerEventsBoxNone, LiveRegion: core.LiveRegionPolite,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), AlignItems: core.AlignCenter, Gap: 2},
		StyleFunc: func() core.Style {
			win := cx.Env().Window()
			// Upright the lines sit in from the picture's sides and over its foot by shares of it, as the design does; on its side, clear of the controls' capsule while it shows.
			side, edge := win.Width*0.04, win.Width/max(core.Points(p.aspect.Get()), 1)*0.08
			if shape() != upright {
				side, edge = win.Width*0.06, 28
				if controlsOver() {
					edge = 74
				}
			}
			st := core.Style{Padding: core.Edges{Left: side, Right: side}, PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea, Right: core.EnvSafeArea}}
			if p.s.Prefs().Subtitles.Top {
				// At the top they keep clear of the row over the picture the same way they keep clear of the capsule at the foot.
				if shape() != upright && controlsOver() {
					edge = 64
				}
				st.Top = core.Pt(edge)
				return st
			}
			st.Bottom = core.Pt(edge)
			return st
		},
		Children: []core.Node{body}}
}

// positionSlider is where the film is, which a drag moves: a thin white line on a faint track under a white knob, without the step marks ui.Slider draws (ui/slider.go:113-128). The design cuts the track at the film's chapters, which the phone does not have (sync.Film has none), so it is one. inset keeps the knob inside the slider's ends, as between the capsule's times; without it the track runs the slider's whole width, as over the upright transport's times, and the knob's half overhangs its ends.
func positionSlider(cx *core.Scope, p *play, inset bool) core.Node {
	seconds := core.NewMemo(cx, func() float64 { return float64(int64(p.length.Get().Seconds())) })
	const height, track, knob = 18, 4, 14
	ends := core.Points(0)
	if inset {
		ends = knob / 2
	}
	return base.Slider{TestID: "player-position", ThumbTestID: "player-position-thumb", Label: locales.PlayerPosition(cx),
		Min:      func() float64 { return 0 },
		Max:      func() float64 { return max(seconds.Get(), 1) },
		Step:     func() float64 { return 1 },
		Value:    func() base.SliderValue { return base.SingleValue(p.at.Get().Seconds()) },
		OnChange: func(v base.SliderValue) { p.seek(time.Duration(v.End * float64(time.Second))) },
		ValueText: func(v float64) string {
			return clock(time.Duration(v * float64(time.Second)))
		},
		Style:          core.Style{Height: core.Pt(height), MinWidth: core.Pt(80), FlexGrow: 1, FlexShrink: 1},
		TrackStyle:     core.Style{Height: core.Pct(100)},
		IndicatorStyle: core.Style{Height: core.Pt(track), CornerRadius: track / 2, Margin: core.Edges{Top: (height - track) / 2, Left: ends, Right: ends}, Background: theme.PlayerInk.Faded(0.2)},
		RangeStyle:     core.Style{CornerRadius: track / 2, Background: theme.PlayerInk},
		ThumbStyle: core.Style{Width: core.Pt(knob), Height: core.Pt(knob), CornerRadius: knob / 2, Background: theme.PlayerInk,
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: 1}, Blur: 4, Color: core.RGBA(0x000000, 0.5)}}},
	}
}

// timeText is a time of the transport in the fixed-pitch face, faded.
func timeText(id string, size core.Points, text func() string) core.Node {
	return core.Text{TestID: id, ContentFunc: text, MaxLines: 1, Style: mono.Merge(core.Style{FontSize: size, LineHeight: size + 4, Color: theme.PlayerInk.Faded(0.55)})}
}

// left writes how much of the film is left to play, as -2:39.
func left(p *play) string { return "-" + clock(max(p.length.Get()-p.at.Get(), 0)) }

// pillFace is the face of a pill of the transport: its text bold on a faint capsule.
func pillFace(text func() string, width, height core.Points) core.Node {
	return core.View{Style: core.Style{MinWidth: core.Pt(width), Height: core.Pt(height), Padding: core.Symmetric(0, 10), CornerRadius: height / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
		Background: theme.PlayerInk.Faded(0.12)}, Children: []core.Node{
		core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightBold, Color: theme.PlayerInk}},
	}}
}

// speedMenu is the speed's pill, which offers the player's speeds.
func speedMenu(cx *core.Scope, p *play, height core.Points) core.Node {
	return ui.Menu{TestID: "player-speed", Label: locales.PlayerSpeed(cx), Presentation: base.PresentSheet,
		Trigger: ui.Button{Variant: ui.ButtonGhost},
		Face:    pillFace(func() string { return speedFace(p.speed()) }, 52, height),
		Children: func() []core.Node {
			var items []core.Node
			for _, r := range Speeds {
				items = append(items, ui.MenuItem{TestID: "player-speed-" + speedText(r), Text: speedText(r), Checked: func() bool { return p.speed() == r }, OnSelect: func() { _ = p.rate.Set(r) }})
			}
			return items
		}()}
}

// glyphButton is a white glyph of the transport in a round target, as the line before and after; disabled, nil for never, dims it and takes its presses.
func glyphButton(id string, label func() string, glyph func() *core.SVG, target, size core.Points, disabled func() bool, press func()) core.Node {
	return base.Button{TestID: id, LabelFunc: label, Disabled: disabled, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Width: core.Pt(target), Height: core.Pt(target), CornerRadius: target / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Active: &core.Style{Opacity: core.Alpha(0.6)}, Disabled: &core.Style{Opacity: core.Alpha(0.35)}},
		Children: []core.Node{ui.IconOf(glyph, size, func() core.Color { return theme.PlayerInk })}}
}

// playGlyphOf is the play or hold glyph, as the film plays or holds.
func playGlyphOf(p *play) func() *core.SVG {
	return func() *core.SVG {
		if p.playing.Get() {
			return pauseGlyph()
		}
		return playGlyph()
	}
}

// playLabel is what the play button says it does.
func playLabel(cx *core.Scope, p *play) func() string {
	return func() string {
		if p.playing.Get() {
			return locales.PlayerPause(cx)
		}
		return locales.PlayerPlay(cx)
	}
}

// transport is the position slider with the time played and the time left under it, and the buttons: the speed, the line before, play or hold, the line after, and the subtitle mode. Looping a line is the card's.
func transport(cx *core.Scope, p *play) core.Node {
	playButton := base.Button{TestID: "player-play", LabelFunc: playLabel(cx, p), Disabled: func() bool { return !p.hasVideo() }, OnPress: func(core.PressEvent) { p.toggle() },
		Style: core.Style{Width: core.Pt(66), Height: core.Pt(66), CornerRadius: 33, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: theme.PlayerInk,
			Active: &core.Style{Opacity: core.Alpha(0.8)}, Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
		Children: []core.Node{ui.IconOf(playGlyphOf(p), 30, func() core.Color { return theme.PlayerOnLight })}}
	mode := ui.Menu{TestID: "player-mode", Label: locales.PlayerSubtitles(cx), Presentation: base.PresentSheet,
		Trigger: ui.Button{Variant: ui.ButtonGhost},
		Face:    pillFace(func() string { return modeShort(cx, p.mode.Get()) }, 52, 30),
		Children: func() []core.Node {
			var items []core.Node
			for _, m := range Modes {
				items = append(items, ui.MenuItem{TestID: "player-mode-" + m, Text: modeName(cx, m), Checked: func() bool { return p.mode.Get() == m }, OnSelect: func() { _ = p.mode.Set(m) }})
			}
			return items
		}()}
	return core.View{TestID: "player-transport", Style: core.Style{Padding: core.Edges{Top: 12, Right: 18, Bottom: 4, Left: 18}, Gap: 6}, Children: []core.Node{
		// The slider's range follows the video's length, which it tells once it has loaded.
		core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{positionSlider(cx, p, false)}},
		core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{
			timeText("player-at", 11, func() string { return clock(p.at.Get()) }),
			timeText("player-left", 11, func() string { return left(p) }),
		}},
		// Play stays in the middle however wide the pills' words are: each side is half of what play leaves, the pill at its outer end and the skip 20 from play, as the design's row of 52, 52, 66, 52 and 52 spread over 354.
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 2}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, Gap: 8, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Padding: core.Edges{Right: 20}}, Children: []core.Node{
				speedMenu(cx, p, 30),
				glyphButton("player-prev", func() string { return locales.PlayerPrevLine(cx) }, skipBackGlyph, 52, 26, nil, p.prev),
			}},
			playButton,
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, Gap: 8, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Padding: core.Edges{Left: 20}}, Children: []core.Node{
				glyphButton("player-next", func() string { return locales.PlayerNextLine(cx) }, skipNextGlyph, 52, 26, nil, p.next),
				mode,
			}},
		}},
	}}
}

// capsule is a phone on its side's controls, in a frosted capsule over the picture's foot: the line before, play or hold, the line after, the time played, the slider and the time left, the loop of the line and the speed.
// The frost is the platform's: Android and Linux draw no backdrop filters (core/style.go:144), so there the capsule is its fill alone.
func capsule(cx *core.Scope, p *play) core.Node {
	looped := func() bool { return p.loop.Get() >= 0 }
	loop := base.Button{TestID: "player-loop", LabelFunc: func() string { return locales.PlayerCapsuleLoop(cx) }, Selected: looped, OnPress: func(core.PressEvent) { p.toggleLoop() },
		Style: core.Style{Height: core.Pt(30), Padding: core.Symmetric(0, 12), CornerRadius: 15, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style {
			if looped() {
				return core.Style{Background: theme.PlayerBlue}
			}
			return core.Style{Background: theme.PlayerInk.Faded(0.14)}
		},
		Children: []core.Node{core.Text{ContentFunc: func() string { return locales.PlayerCapsuleLoop(cx) }, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightBold, Color: theme.PlayerInk}}}}
	return core.View{TestID: "player-transport",
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Height: core.Pt(50), Padding: core.Symmetric(0, 10), CornerRadius: 25, BorderCurve: core.BorderCurveContinuous,
			Background: theme.PlayerGlass.Faded(0.55), BackdropFilters: []core.Filter{core.Blur(20), core.Saturate(1.6)},
			Shadows: []core.BoxShadow{{Spread: 0.5, Color: theme.PlayerInk.Faded(0.15), Inset: true}}},
		Children: []core.Node{
			glyphButton("player-prev", func() string { return locales.PlayerPrevLine(cx) }, skipBackGlyph, 36, 20, nil, p.prev),
			glyphButton("player-play", playLabel(cx, p), playGlyphOf(p), 38, 24, func() bool { return !p.hasVideo() }, p.toggle),
			glyphButton("player-next", func() string { return locales.PlayerNextLine(cx) }, skipNextGlyph, 36, 20, nil, p.next),
			timeText("player-at", 12, func() string { return clock(p.at.Get()) }),
			positionSlider(cx, p, true),
			timeText("player-left", 12, func() string { return left(p) }),
			loop,
			speedMenu(cx, p, 30),
		}}
}

// modeShort is a subtitle mode's short name, on the buttons that show the mode: the transport's pill and the pill over the picture of a phone on its side.
func modeShort(cx *core.Scope, mode string) string {
	switch mode {
	case kcore.SubOriginal:
		return locales.PlayerModeShortOriginal(cx)
	case kcore.SubTranslation:
		return locales.PlayerModeShortTranslation(cx)
	case kcore.SubOff:
		return locales.PlayerModeShortHidden(cx)
	}
	return locales.PlayerModeShortBilingual(cx)
}

// modeName is what the player calls a subtitle mode.
func modeName(cx *core.Scope, mode string) string {
	switch mode {
	case kcore.SubOriginal:
		return locales.PlayerModeOriginal(cx)
	case kcore.SubTranslation:
		return locales.PlayerModeTranslation(cx)
	case kcore.SubOff:
		return locales.PlayerModeHidden(cx)
	}
	return locales.PlayerModeBilingual(cx)
}

// lines is the list of the film's lines: each with its start and its texts as the mode picks them, the original first, the one playing on a light band in bold, those before it faded, the one looped barred and the saved starred; a tap plays from a line. It follows the line playing unless the user is scrolling it. compact is the narrower list at the side of a phone on its side.
func lines(cx *core.Scope, p *play, compact bool) core.Node {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	dragging := core.NewSignal(cx, false)
	cx.Effect(func() {
		if i := p.current.Get(); i >= 0 && !dragging.GetUntracked() {
			ref.ScrollToIndexAt(i, 0.3, 0, true)
		}
	})
	empty := core.Show{When: func() bool { _, ok := p.cues.Get(); return ok && len(p.list()) == 0 }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(24), AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Text{TestID: "player-no-lines", ContentFunc: func() string { return locales.PlayerNoLines(cx) }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)}}}
	}}
	first, second := core.Style{FontSize: 15, LineHeight: 21}, core.Style{FontSize: 13, LineHeight: 18}
	if compact {
		first, second = core.Style{FontSize: 14, LineHeight: 19}, core.Style{FontSize: 12, LineHeight: 16}
	}
	list := core.List{TestID: "player-lines", Ref: ref, EstimatedSize: 64,
		Style:             core.Style{FlexGrow: 1, FlexShrink: 1},
		Count:             func() int { return len(p.list()) },
		Key:               func(i int) any { return i },
		OnScrollDragStart: func(core.ScrollEvent) { dragging.Set(true) },
		OnScrollEnd:       func(core.ScrollEvent) { dragging.Set(false) },
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			cue := func() cues.Cue {
				if l := p.list(); row.Index() < len(l) {
					return l[row.Index()]
				}
				return cues.Cue{}
			}
			texts := core.NewMemoFunc(cx, func() []kcore.CueLine {
				c := cue()
				return kcore.CueLines(p.mode.Get(), kcore.SubOriginalFirst, c.Translation, c.Original)
			}, slices.Equal[[]kcore.CueLine])
			playing := func() bool { return p.current.Get() == row.Index() }
			looped := func() bool { return p.loop.Get() == row.Index() }
			saved := func() bool { return p.isSaved(row.Index()) }
			body := core.Dynamic(func(cx *core.Scope) core.Node {
				var children []core.Node
				all := texts.Get()
				for k, l := range all {
					st, color := first.Merge(core.Style{FontWeight: core.FontWeightMedium}), colors.Text
					if k > 0 && len(all) > 1 {
						st, color = second, func() core.Color { return ui.Alpha(colors.Text(), 0.6) }
					}
					weight := st.FontWeight
					children = append(children, core.Text{Content: l.Text, Style: st, StyleFunc: func() core.Style {
						s := core.Style{Color: color()}
						if k == 0 && playing() {
							s.FontWeight = core.FontWeightBold
						} else {
							s.FontWeight = weight
						}
						return s
					}})
				}
				return core.Group(children)
			})
			return core.View{TestID: "player-line", Role: core.RoleButton,
				Selected: playing,
				OnPress:  func(core.PressEvent) { p.step(row.Index()) },
				Style:    core.Style{Direction: core.Row, Gap: 10, Padding: core.All(10), Margin: core.Edges{Left: 8, Right: 8}, CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
				StyleFunc: func() core.Style {
					st := core.Style{Active: &core.Style{Background: colors.Pressed()}}
					if playing() {
						st.Background = theme.PlayerInk.Faded(0.13)
					} else if c := p.current.Get(); c >= 0 && row.Index() < c {
						st.Opacity = core.Alpha(0.5)
					}
					return st
				},
				Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(34), FlexShrink: 0, Direction: core.Row, Gap: 3}, Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(2), CornerRadius: 1}, StyleFunc: func() core.Style {
							if looped() {
								return core.Style{Background: theme.PlayerBlue}
							}
							return core.Style{}
						}},
						core.Text{ContentFunc: func() string { return clock(cue().Start) }, MaxLines: 1, Style: mono.Merge(core.Style{FontSize: 11, LineHeight: 21, Color: theme.PlayerInk.Faded(0.5)})},
					}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{body}},
					core.Show{When: saved, Then: func(cx *core.Scope) core.Node {
						return core.View{TestID: "player-line-saved", Style: core.Style{Padding: core.Edges{Top: 4}}, Children: []core.Node{ui.IconOf(starFilled, 13, func() core.Color { return originalColor })}}
					}},
				}}
		}}
	return core.View{Role: core.RoleGroup, LabelFunc: func() string { return locales.PlayerLines(cx) }, Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.Edges{Top: 6}}, Children: []core.Node{empty, list}}
}
