package learn

import (
	"strings"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// dictationCard dictates clip's line: a press plays it from the film kept on the phone, as many times as the learner likes, they type what they heard, and Check, which calls check, marks each word of the line they typed or missed, with the line's translation, once checked reports true. It is built anew for each card, so what was typed goes with the card.
func dictationCard(cx *core.Scope, clip learning.Clip, checked func() bool, check func()) core.Node {
	colors, own := ui.ColorsOf(cx), theme.Of(cx)
	typed := core.NewSignal(cx, "")
	playing := core.NewSignal(cx, false)
	failed := core.NewSignal(cx, false)
	var player *audio.Player
	stop := func() {
		player.Pause()
		player.Seek(clip.From)
		playing.Set(false)
	}
	player = audio.NewPlayer(cx, audio.Source{URI: clip.Video}, audio.PlayerOptions{
		AudioSession: core.AudioSessionPlayback,
		OnProgress: func(at, _ time.Duration) {
			if playing.GetUntracked() && at >= clip.To {
				stop()
			}
		},
		OnEnd:   func() { playing.Set(false) },
		OnError: func(error) { playing.Set(false); failed.Set(true) },
	})
	play := func() {
		if playing.GetUntracked() {
			stop()
			return
		}
		failed.Set(false)
		playing.Set(true)
		player.Seek(clip.From)
		player.Play(nil)
	}
	result := core.NewMemoFunc(cx, func() dictated {
		words, hit, total := learning.Dictate(clip.Line, typed.Get())
		return dictated{words: words, hit: hit, total: total}
	}, nil)
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14}, Children: []core.Node{
		// The play button is the design's rounded square on the accent, with a filled triangle.
		base.Button{TestID: "dictation-play", OnPress: func(core.PressEvent) { play() },
			LabelFunc: func() string {
				if playing.Get() {
					return locales.DictationStop(cx)
				}
				return locales.DictationPlay(cx)
			},
			Style: core.Style{Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style { return core.Style{Background: colors.Accent(), FocusVisible: ui.FocusRing(cx, -2)} },
			Children: []core.Node{ui.Icon(func() *core.SVG {
				if playing.Get() {
					return lucide.Pause()
				}
				return playIcon()
			}, 24, colors.OnAccent)}},
		core.View{Style: core.Style{Gap: 2, FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.DictationTitle(cx) }, Role: core.RoleHeader, Style: core.Style{FontSize: 18, LineHeight: 23, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{TestID: "dictation-hint", ContentFunc: func() string {
				if failed.Get() {
					return locales.DictationCantPlay(cx)
				}
				return locales.DictationHint(cx)
			}, Style: look.Caption, StyleFunc: func() core.Style {
				if failed.Get() {
					return core.Style{Color: colors.Danger()}
				}
				return core.Style{Color: colors.Muted()}
			}},
		}},
	}}
	input := ui.Input{TestID: "dictation-input", Multiline: true, LabelFunc: func() string { return locales.DictationLabel(cx) },
		PlaceholderFunc: func() string { return locales.DictationPlaceholder(cx) },
		Value:           typed.Get, OnChange: typed.Set, Disabled: checked,
		// The field is the fill's grey with no rim and a placeholder in the muted grey, as the design draws it; the caret stays ui.Input's.
		Style: core.Style{FontSize: 17, LineHeight: 24, CornerRadius: 14, Height: core.Pt(96), Padding: core.Symmetric(12, 14)},
		StyleFunc: func() core.Style {
			none := ui.Alpha(colors.Inset(), 0)
			return core.Style{Background: colors.Inset(), BorderColor: none, Hover: &core.Style{BorderColor: none}, PlaceholderColor: colors.Muted()}
		}}
	checkButton := core.Show{When: func() bool { return !checked() }, Then: func(cx *core.Scope) core.Node {
		text := func() string { return locales.DictationCheck(cx) }
		return base.Button{TestID: "dictation-check", LabelFunc: text, OnPress: func(core.PressEvent) { check() },
			Style: core.Style{Height: core.Pt(46), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style {
				return core.Style{Background: colors.Text(), Color: colors.Background(), FocusVisible: ui.FocusRing(cx, -2)}
			},
			Children: []core.Node{core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold}}}}
	}}
	marked := core.Show{When: checked, Then: func(cx *core.Scope) core.Node {
		var words []core.Node
		for _, w := range result.GetUntracked().words {
			st := core.Style{Padding: core.Symmetric(1, 3), CornerRadius: 5, FontSize: 18, LineHeight: 23}
			switch {
			case !w.Counted:
				st.Color = colors.Text()
			case w.Typed:
				st.Color, st.Background = own.OkText(), ui.Alpha(theme.StatusOK, 0.14)
			default:
				st.Color, st.Background = colors.Danger(), ui.Alpha(theme.BadgeRed, 0.12)
			}
			words = append(words, core.Text{Content: w.Text, Style: st})
		}
		return core.View{TestID: "dictation-result", Style: core.Style{Gap: 8}, Children: []core.Node{
			ui.Tabular(core.Text{TestID: "dictation-score", LiveRegion: core.LiveRegionPolite,
				ContentFunc: func() string {
					r := result.Get()
					if learning.ByCharacter(clip.Line) {
						return locales.DictationScoreChars(cx, r.hit, r.total)
					}
					return locales.DictationScore(cx, r.hit, r.total)
				},
				Style:       look.Caption.Merge(core.Style{FontWeight: core.FontWeightBold}), StyleFunc: ui.Colored(colors.Muted)}),
			// The marked words sit close, on lines 30 apart, as the design sets them inline.
			core.View{TestID: "dictation-line", Style: core.Style{Direction: core.Row, Wrap: core.Wrap, ColumnGap: 2, RowGap: 5}, Children: words},
			core.Show{When: func() bool { return strings.TrimSpace(clip.Translation) != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "dictation-translation", Content: clip.Translation, Style: core.Style{FontSize: 14, LineHeight: 20}, StyleFunc: ui.Colored(colors.Muted)}
			}},
		}}
	}}
	return core.View{TestID: "dictation-card", Style: core.Style{Gap: 18, CornerRadius: 26, BorderCurve: core.BorderCurveContinuous, Padding: core.Symmetric(22, 20),
		Shadows: []core.BoxShadow{{Offset: core.Point{Y: 20}, Blur: 50, Color: theme.CardShadow}}},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} },
		Children:  []core.Node{header, input, checkButton, marked}}
}

// dictated is a check of what was typed: the line's words, and how many of those counted were typed out of how many.
type dictated struct {
	words      []learning.Heard
	hit, total int
}
