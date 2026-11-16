package player

import (
	"slices"
	"time"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The faces of the lines over the video, as the desktop's default look has them: the translation bold in white over the original in yellow, each on a dark box, in sizes for the phone's screen by layout.
var (
	translationColor = core.RGB(0xFFFFFF)
	originalColor    = core.RGB(0xFFD60A)
	lineBox          = core.RGBA(0x000000, 0.62)
)

// lineSizes are the sizes of the translation and the original by layout.
func lineSizes(l layout) (translation, original core.Points) {
	switch l {
	case full:
		return 24, 19
	case side:
		return 22, 17
	}
	return 17, 14
}

// subtitles draws the line playing over the video, as the mode picks its texts, up out of the transport's way while it shows over the video.
func subtitles(cx *core.Scope, p *play, shape func() layout, controlsOver func() bool) core.Node {
	shown := core.NewMemoFunc(cx, func() []kcore.CueLine {
		i, list := p.current.Get(), p.list()
		if i < 0 || i >= len(list) {
			return nil
		}
		return kcore.PictureLines(p.mode.Get(), kcore.SubTranslationFirst, kcore.UntranslatedDim, list[i].Translation, list[i].Original)
	}, slices.Equal[[]kcore.CueLine])
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		tr, orig := lineSizes(shape())
		var children []core.Node
		for _, l := range shown.Get() {
			size, color, weight := tr, translationColor, core.FontWeightBold
			if l.Original {
				size, color, weight = orig, originalColor, core.FontWeightMedium
			}
			holder := core.Style{MaxWidth: core.Pct(100), CornerRadius: 6, Padding: core.Symmetric(2, 10), Background: lineBox}
			if l.Dim {
				holder.Opacity = core.Alpha(0.6)
			}
			children = append(children, core.View{Style: holder, Children: []core.Node{
				core.Text{Content: l.Text, Style: core.Style{FontSize: size, LineHeightMultiple: 1.25, FontWeight: weight, Color: color, TextAlign: core.TextAlignCenter}},
			}})
		}
		return core.Group(children)
	})
	return core.View{TestID: "player-subtitles", PointerEvents: core.PointerEventsNone, LiveRegion: core.LiveRegionPolite,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), AlignItems: core.AlignCenter, Gap: 4, Padding: core.Symmetric(0, 16)},
		StyleFunc: func() core.Style {
			bottom := core.Points(10)
			if shape() == full {
				bottom = 28
				if controlsOver() {
					bottom = 120
				}
			}
			return core.Style{Bottom: core.Pt(bottom), PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea, Right: core.EnvSafeArea}}
		},
		Children: []core.Node{body}}
}

// transport is the position slider with the times, and the buttons: the line before, play or hold, the line after, loop the line, the speed and the subtitle mode.
func transport(cx *core.Scope, p *play) core.Node {
	colors := ui.ColorsOf(cx)
	length := func() time.Duration { return p.length.Get() }
	seconds := core.NewMemo(cx, func() float64 { return float64(int64(length().Seconds())) })
	times := func(d func() time.Duration, id string) core.Node {
		return ui.Tabular(core.Text{TestID: id, ContentFunc: func() string { return clock(d()) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)})
	}
	icon := func(id string, label func() string, svg func() *core.SVG, press func()) core.Node {
		return ui.IconButton{TestID: id, Icon: svg, LabelFunc: label, Variant: ui.IconButtonGhost, Size: 44, IconSize: 22, OnPress: press}
	}
	playButton := ui.IconButton{TestID: "player-play", Variant: ui.IconButtonGhost, Size: 52, IconSize: 30,
		IconFunc: func() *core.SVG {
			if p.playing.Get() {
				return lucide.Pause()
			}
			return lucide.Play()
		},
		LabelFunc: func() string {
			if p.playing.Get() {
				return locales.PlayerPause(cx)
			}
			return locales.PlayerPlay(cx)
		},
		OnPress: p.toggle}
	loop := ui.IconButton{TestID: "player-loop", Icon: lucide.Repeat1, LabelFunc: func() string { return locales.PlayerLoopLine(cx) }, Variant: ui.IconButtonGhost, Size: 44, IconSize: 22,
		Selected: func() bool { return p.loop.Get() >= 0 }, OnPress: p.toggleLoop}
	speed := ui.Menu{TestID: "player-speed", Label: locales.PlayerSpeed(cx), Presentation: base.PresentSheet,
		Trigger: ui.Button{Variant: ui.ButtonGhost},
		Face:    ui.Tabular(core.Text{ContentFunc: func() string { return speedText(p.speed()) }, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightSemibold, MinWidth: core.Pt(44), TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)}),
		Children: func() []core.Node {
			var items []core.Node
			for _, r := range Speeds {
				items = append(items, ui.MenuItem{TestID: "player-speed-" + speedText(r), Text: speedText(r), Checked: func() bool { return p.speed() == r }, OnSelect: func() { _ = p.rate.Set(r) }})
			}
			return items
		}()}
	mode := ui.Menu{TestID: "player-mode", Label: locales.PlayerSubtitles(cx), Presentation: base.PresentSheet,
		Trigger: ui.Button{Variant: ui.ButtonGhost},
		Face:    core.Icon{SVG: lucide.Captions(), Style: core.Style{Width: core.Pt(22), Height: core.Pt(22)}, StyleFunc: ui.Colored(colors.Text)},
		Children: func() []core.Node {
			var items []core.Node
			for _, m := range Modes {
				items = append(items, ui.MenuItem{TestID: "player-mode-" + m, Text: modeName(cx, m), Checked: func() bool { return p.mode.Get() == m }, OnSelect: func() { _ = p.mode.Set(m) }})
			}
			return items
		}()}
	return core.View{TestID: "player-transport", Style: core.Style{Padding: core.Edges{Top: 6, Right: 12, Bottom: 6, Left: 12}, Gap: 2}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			times(func() time.Duration { return p.at.Get() }, "player-at"),
			// The slider's range is fixed when it is built, so it is built again once the video tells its length.
			// TODO(nsgo): give ui.Slider a MaxFunc twin of Max, so a range that is known only once a video loads needs no rebuild
			core.View{Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0)}, Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
				return ui.Slider{TestID: "player-position", Label: locales.PlayerPosition(cx), Min: 0, Step: 1, Max: max(seconds.Get(), 1),
					Value:     func() float64 { return p.at.Get().Seconds() },
					OnChange:  func(v float64) { p.seek(time.Duration(v * float64(time.Second))) },
					ValueText: func(v float64) string { return clock(time.Duration(v * float64(time.Second))) }}
			})}},
			times(length, "player-length"),
		}},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{
			speed,
			icon("player-prev", func() string { return locales.PlayerPrevLine(cx) }, lucide.SkipBack, func() { p.step(cues.Prev(p.list(), p.at.GetUntracked())) }),
			playButton,
			icon("player-next", func() string { return locales.PlayerNextLine(cx) }, lucide.SkipForward, func() { p.step(cues.Next(p.list(), p.at.GetUntracked())) }),
			loop,
			mode,
		}},
	}}
}

// modeName is what the player calls a subtitle mode.
func modeName(cx *core.Scope, mode string) string {
	switch mode {
	case kcore.SubOriginal:
		return locales.PlayerModeOriginal(cx)
	case kcore.SubTranslation:
		return locales.PlayerModeTranslation(cx)
	}
	return locales.PlayerModeBilingual(cx)
}

// lines is the list of the film's lines: each with its start and its texts as the mode picks them, the one playing marked, the one looped barred; a tap plays from a line. It follows the line playing unless the user is scrolling it.
func lines(cx *core.Scope, p *play) core.Node {
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
				return kcore.CueLines(p.mode.Get(), kcore.SubTranslationFirst, c.Translation, c.Original)
			}, slices.Equal[[]kcore.CueLine])
			playing := func() bool { return p.current.Get() == row.Index() }
			looped := func() bool { return p.loop.Get() == row.Index() }
			body := core.Dynamic(func(cx *core.Scope) core.Node {
				var children []core.Node
				for _, l := range texts.Get() {
					st := look.Body
					color := colors.Text
					if l.Original && len(texts.Get()) > 1 {
						st, color = look.Caption.Merge(core.Style{FontSize: 14, LineHeight: 19}), colors.Muted
					}
					children = append(children, core.Text{Content: l.Text, Style: st, StyleFunc: ui.Colored(color)})
				}
				return core.Group(children)
			})
			return core.View{TestID: "player-line", Role: core.RoleButton,
				Selected: playing,
				OnPress:  func(core.PressEvent) { p.step(row.Index()) },
				Style:    core.Style{Direction: core.Row, Gap: 12, Padding: core.Symmetric(10, 16)},
				StyleFunc: func() core.Style {
					st := core.Style{Active: &core.Style{Background: colors.Pressed()}}
					if playing() {
						st.Background = colors.Accent().Faded(0.22)
					}
					return st
				},
				Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(3), CornerRadius: 1.5}, StyleFunc: func() core.Style {
						if looped() {
							return core.Style{Background: originalColor}
						}
						return core.Style{}
					}},
					ui.Tabular(core.Text{ContentFunc: func() string { return clock(cue().Start) }, Style: look.Caption.Merge(core.Style{Width: core.Pt(44)}), StyleFunc: ui.Colored(colors.Muted)}),
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{body}},
				}}
		}}
	return core.View{Role: core.RoleGroup, LabelFunc: func() string { return locales.PlayerLines(cx) }, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{empty, list}}
}
