package player

import (
	"cmp"
	"slices"
	"strings"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"
	kstudy "kando/packages/study"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// filmWord is a word of the film as the Words tab lists it: the word as the vocabulary lists it, its dictionary form, else as tapped; tapped as it reads in the line it was looked up in, line, -1 when the film has that line no more; what the computer answered of it; how many times it is said in the film; and whether it is in the vocabulary.
type filmWord struct {
	word, tapped string
	entry        sync.WordEntry
	line         int
	count        int
	saved        bool
}

// filmWords are the words of the film: those looked up in its lines on the phone and those of the vocabulary looked up in it, on any device, each once, in the order the film says them. Read it in a binding.
func (p *play) filmWords() []filmWord {
	list := p.list()
	if len(list) == 0 {
		return nil
	}
	f, _ := p.s.Film(p.key)
	originals := make([]string, len(list))
	for i, c := range list {
		originals[i] = strings.TrimSpace(c.Original)
	}
	lineOf := func(original string) int { return slices.Index(originals, strings.TrimSpace(original)) }
	byKey := map[string]int{}
	var out []filmWord
	add := func(w filmWord) {
		k := kstudy.WordKey(w.word)
		if k == "" {
			return
		}
		if i, ok := byKey[k]; ok {
			out[i].saved = out[i].saved || w.saved
			return
		}
		byKey[k] = len(out)
		out = append(out, w)
	}
	vocab := p.s.Vocabulary(p.key)
	inVocab := func(word string) bool {
		k := kstudy.WordKey(word)
		return slices.ContainsFunc(vocab, func(it learning.Item) bool { return kstudy.WordKey(it.Text) == k })
	}
	for _, l := range p.s.LookedUp(p.key, originals) {
		word := cmp.Or(strings.TrimSpace(l.Entry.Word), l.Word)
		add(filmWord{word: word, tapped: l.Word, entry: l.Entry, line: lineOf(l.Line), saved: inVocab(word)})
	}
	for _, it := range vocab {
		if it.Film != f.Film.ID {
			continue
		}
		line := lineOf(it.Context)
		if line < 0 {
			line = cues.Index(list, it.Start)
		}
		add(filmWord{word: it.Text, tapped: it.Text, entry: sync.WordEntry{Word: it.Text, Reading: it.Reading, Meaning: it.Meaning}, line: line, saved: true})
	}
	for i := range out {
		out[i].count = timesSaid(p.tokens, originals, out[i].tapped, out[i].word)
	}
	slices.SortStableFunc(out, func(a, b filmWord) int {
		// A word whose line is gone goes last.
		la, lb := a.line, b.line
		if la < 0 {
			la = len(list)
		}
		if lb < 0 {
			lb = len(list)
		}
		return la - lb
	})
	return out
}

// timesSaid is how many times lines say one of words, in whatever case, each line cut into its words by tokens.
func timesSaid(tokens func(string) []kcore.WordToken, lines []string, words ...string) int {
	var keys []string
	for _, w := range words {
		if k := kstudy.WordKey(w); k != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	n := 0
	for _, l := range lines {
		for _, tok := range tokens(l) {
			if tok.Word && slices.Contains(keys, kstudy.WordKey(tok.Text)) {
				n++
			}
		}
	}
	return n
}

// inVocabulary reports whether the word looked up in the card is in the vocabulary. Read it in a binding.
func (w *words) inVocabulary() bool {
	l := w.lookup.Get()
	_, ok := w.p.s.VocabularyWord(w.p.key, cmp.Or(strings.TrimSpace(l.entry.Word), l.word))
	return ok
}

// toggleWord puts word, tapped in line i as e answers it, in the vocabulary, or takes it out, and says which.
func (p *play) toggleWord(word string, e sync.WordEntry, i int) {
	if !p.s.Studyable(p.key) {
		p.say("word", locales.PlayerCannotAddWord(p.cx))
		return
	}
	shown := cmp.Or(strings.TrimSpace(e.Word), strings.TrimSpace(word))
	if p.s.ToggleWord(p.key, word, e, p.studyCue(i)) {
		p.say("word", locales.PlayerWordAdded(p.cx, shown))
		return
	}
	p.say("word", locales.PlayerWordRemoved(p.cx, shown))
}

// vocabButton is the button of the word card that puts the word looked up in the vocabulary, or takes it out, once the computer has answered.
func vocabButton(cx *core.Scope, p *play, w *words) core.Node {
	press := func() {
		l := w.lookup.GetUntracked()
		p.toggleWord(l.word, l.entry, l.line)
	}
	return core.Show{When: func() bool { l := w.lookup.Get(); return !l.busy && !l.failed && l.entry.Meaning != "" }, Then: func(cx *core.Scope) core.Node {
		return core.Show{When: w.inVocabulary,
			Then: func(cx *core.Scope) core.Node {
				return look.Button{TestID: "player-word-vocab", Kind: look.Tinted, Icon: lucide.Check, Text: func() string { return locales.PlayerWordInVocabulary(cx) }, OnPress: press}
			},
			Else: func(cx *core.Scope) core.Node {
				return look.Button{TestID: "player-word-vocab", Kind: look.Primary, Icon: lucide.Plus, Text: func() string { return locales.PlayerWordAdd(cx) }, OnPress: press}
			}}
	}}
}

// vocabStar is the star of the word card at the corner of the picture that puts the word looked up in the vocabulary, or takes it out, once the computer has answered.
func vocabStar(cx *core.Scope, p *play, w *words) core.Node {
	return core.Show{When: func() bool { l := w.lookup.Get(); return !l.busy && !l.failed && l.entry.Meaning != "" }, Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		return ui.IconButton{TestID: "player-word-vocab", Icon: lucide.Star, Variant: ui.IconButtonGhost, Size: 36, Selected: w.inVocabulary,
			Tint: func() core.Color { return starTint(colors, w.inVocabulary()) },
			LabelFunc: func() string {
				if w.inVocabulary() {
					return locales.PlayerWordInVocabulary(cx)
				}
				return locales.PlayerWordAdd(cx)
			},
			OnPress: func() {
				l := w.lookup.GetUntracked()
				p.toggleWord(l.word, l.entry, l.line)
			}}
	}}
}

// starTint is the color of a word's star: the yellow of the saved while it is in the vocabulary.
func starTint(colors ui.Colors, saved bool) core.Color {
	if saved {
		return theme.SubtitleYellow
	}
	return colors.Muted()
}

// wordsPage is the Words tab: the words of the film looked up or in the vocabulary, each with its reading, its meaning and how many times the film says it, and a star that puts it in the vocabulary or takes it out; a tap shows its card.
func wordsPage(cx *core.Scope, p *play, w *words) core.Node {
	colors := ui.ColorsOf(cx)
	all := func() []filmWord { return p.filmWords() }
	return core.Scroll{TestID: "player-words", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.All(16), Gap: 10}, Children: []core.Node{
			core.Show{When: func() bool { return len(all()) == 0 }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "player-words-empty", ContentFunc: func() string { return locales.PlayerWordsEmpty(cx) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
			}},
			core.Each(all, func(fw filmWord) string { return kstudy.WordKey(fw.word) }, func(cx *core.Scope, fw func() filmWord, _ func() int) core.Node {
				return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 10, Right: 6, Bottom: 10, Left: 12}, CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
					StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.05)} },
					Children: []core.Node{
						core.View{TestID: "player-film-word", Role: core.RoleButton, LabelFunc: func() string { return fw().word },
							OnPress: func(core.PressEvent) {
								if x := fw(); x.line >= 0 {
									w.look(x.line, x.tapped)
								}
							},
							Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2},
							Children: []core.Node{
								core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{
									core.Text{ContentFunc: func() string { return fw().word }, MaxLines: 1, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightBold, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
									core.Text{ContentFunc: func() string { return fw().entry.Reading }, MaxLines: 1, Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
								}},
								core.Show{When: func() bool { return fw().entry.Meaning != "" }, Then: func(cx *core.Scope) core.Node {
									return core.Text{ContentFunc: func() string { return fw().entry.Meaning }, MaxLines: 2, Style: look.Subhead, StyleFunc: ui.Colored(colors.Text)}
								}},
								core.Show{When: func() bool { return fw().count > 0 }, Then: func(cx *core.Scope) core.Node {
									return core.Text{ContentFunc: func() string { return locales.PlayerWordTimes(cx, fw().count) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}
								}},
							}},
						ui.IconButton{TestID: "player-film-word-star", Icon: lucide.Star, Variant: ui.IconButtonGhost, Size: 40,
							Selected: func() bool { return fw().saved },
							Tint:     func() core.Color { return starTint(colors, fw().saved) },
							LabelFunc: func() string {
								if fw().saved {
									return locales.PlayerWordRemoveNamed(cx, fw().word)
								}
								return locales.PlayerWordAddNamed(cx, fw().word)
							},
							OnPress: func() {
								x := fw()
								p.toggleWord(x.tapped, x.entry, x.line)
							}},
					}}
			}),
		}},
	}}
}
