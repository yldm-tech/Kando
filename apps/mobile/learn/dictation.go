package learn

import (
	"strings"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// dictationCard dictates clip's line: a press plays it from the film kept on the phone, as many times as the learner likes, they type what they heard, and Check marks each word of the line they typed or missed, with the line's translation, once checked reports true. It is built anew for each card, so what was typed goes with the card.
func dictationCard(cx *core.Scope, clip learning.Clip, checked func() bool) core.Node {
	colors := ui.ColorsOf(cx)
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
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
		ui.IconButton{TestID: "dictation-play", Variant: ui.IconButtonPrimary, Size: 56, IconSize: 24,
			Icon: func() *core.SVG {
				if playing.Get() {
					return lucide.Pause()
				}
				return lucide.Play()
			},
			LabelFunc: func() string {
				if playing.Get() {
					return locales.DictationStop(cx)
				}
				return locales.DictationPlay(cx)
			},
			OnPress: play},
		core.View{Style: core.Style{Gap: 2, FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.DictationTitle(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
			core.Text{TestID: "dictation-hint", ContentFunc: func() string {
				if failed.Get() {
					return locales.DictationCantPlay(cx)
				}
				return locales.DictationHint(cx)
			}, Style: look.Subhead, StyleFunc: func() core.Style {
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
		Style: core.Style{FontSize: 17, CornerRadius: 12}}
	marked := core.Show{When: checked, Then: func(cx *core.Scope) core.Node {
		var words []core.Node
		for _, w := range result.GetUntracked().words {
			st := core.Style{Padding: core.Symmetric(1, 3), CornerRadius: 5}
			switch {
			case !w.Counted:
				st.Color = colors.Text()
			case w.Typed:
				st.Color, st.Background = colors.Up(), ui.Alpha(colors.Up(), 0.14)
			default:
				st.Color, st.Background = colors.Danger(), ui.Alpha(colors.Danger(), 0.12)
			}
			words = append(words, core.Text{Content: w.Text, Style: look.Body.Merge(st)})
		}
		return core.View{TestID: "dictation-result", Style: core.Style{Gap: 8}, Children: []core.Node{
			ui.Tabular(core.Text{TestID: "dictation-score", LiveRegion: core.LiveRegionPolite,
				ContentFunc: func() string { r := result.Get(); return locales.DictationScore(cx, r.hit, r.total) },
				Style:       look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Muted)}),
			core.View{TestID: "dictation-line", Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 4}, Children: words},
			core.Show{When: func() bool { return strings.TrimSpace(clip.Translation) != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "dictation-translation", Content: clip.Translation, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
			}},
		}}
	}}
	return core.View{TestID: "dictation-card", Style: core.Style{Gap: 16, CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, Padding: core.All(20)},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.045)} },
		Children:  []core.Node{header, input, marked}}
}

// dictated is a check of what was typed: the line's words, and how many of those counted were typed out of how many.
type dictated struct {
	words      []learning.Heard
	hit, total int
}
