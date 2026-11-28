package player

import (
	"cmp"
	"slices"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"
	"kando/packages/core/segment"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// lookup is a word looked up: the word as tapped, the line it was tapped in, busy while the computer is asked, then what it answered, or why it could not.
type lookup struct {
	open    bool
	word    string
	line    int
	busy    bool
	entry   sync.WordEntry
	failed  bool
	problem session.StudyProblem
}

// reading is how the word looked up is read, as the answer gives it, or "" when that is the headword itself, as a word written in kana alone, which the sheet would otherwise say twice.
func (l lookup) reading() string {
	if r := l.entry.Reading; r != cmp.Or(l.entry.Word, l.word) {
		return r
	}
	return ""
}

// explanation is a line explained: the line, busy while the computer is asked, then what it answered, or why it could not; more once the sections kept for More show, and the follow-up questions asked about it.
type explanation struct {
	open    bool
	line    int
	busy    bool
	answer  sync.Explanation
	failed  bool
	problem session.StudyProblem
	more    bool
	asked   []followUp
}

// followUp is a follow-up question about the line explained, the n-th asked: its answer once it came, busy while the computer is asked, failed with why when none came.
type followUp struct {
	n                int
	question, answer string
	busy, failed     bool
	problem          session.StudyProblem
}

// words is the word looked up and the line explained, each asked for through the film's computer; gen numbers the asks, so the answer to one the learner left is dropped.
type words struct {
	cx      *core.Scope
	p       *play
	lookup  *core.Signal[lookup]
	explain *core.Signal[explanation]
	gen     int
}

func newWords(cx *core.Scope, p *play) *words {
	// An explanation holds slices, so every Set notifies.
	return &words{cx: cx, p: p, lookup: core.NewSignal(cx, lookup{}), explain: core.NewSignalFunc(cx, explanation{}, nil)}
}

// look looks word up as it reads in line i: the film holds while the card shows it.
func (w *words) look(i int, word string) {
	word = strings.TrimSpace(word)
	if word == "" || i < 0 || i >= len(w.p.list()) {
		return
	}
	if w.p.playing.GetUntracked() {
		w.p.toggle()
	}
	w.gen++
	gen := w.gen
	w.lookup.Set(lookup{open: true, word: word, line: i, busy: true})
	w.p.noteStudy()
	w.p.s.LookUp(w.p.key, word, w.p.list()[i].Original, func(e sync.WordEntry, err error) {
		if gen != w.gen || !w.lookup.GetUntracked().open {
			return
		}
		l := w.lookup.GetUntracked()
		l.busy = false
		if err != nil {
			l.failed, l.problem = true, session.StudyProblemOf(err)
		} else {
			l.entry = e
		}
		w.lookup.Set(l)
	})
}

// closeLookup closes the word looked up, dropping the answer on its way.
func (w *words) closeLookup() {
	w.gen++
	w.lookup.Set(lookup{})
}

// explainLine has line i explained: the film holds while the sheet shows it.
func (w *words) explainLine(i int) {
	if i < 0 || i >= len(w.p.list()) {
		return
	}
	if w.p.playing.GetUntracked() {
		w.p.toggle()
	}
	w.gen++
	gen := w.gen
	w.lookup.Set(lookup{})
	w.explain.Set(explanation{open: true, line: i, busy: true})
	lines, at := w.p.around(i)
	w.p.noteStudy()
	w.p.s.Explain(w.p.key, lines, at, func(x sync.Explanation, err error) {
		if gen != w.gen || !w.explain.GetUntracked().open {
			return
		}
		e := w.explain.GetUntracked()
		e.busy = false
		if err != nil {
			e.failed, e.problem = true, session.StudyProblemOf(err)
		} else {
			e.answer = x
		}
		w.explain.Set(e)
	})
}

// closeExplain closes the line explained, dropping the answers on their way.
func (w *words) closeExplain() {
	w.gen++
	w.explain.Set(explanation{})
}

// expandExplain shows the sections the explanation keeps for More: the tone and related expressions.
func (w *words) expandExplain() {
	e := w.explain.GetUntracked()
	e.more = true
	w.explain.Set(e)
}

// ask asks the computer question about the line explained, with the line, the lines around it, of which the computer sends only those before it, what was explained of it and the questions answered before. The question shows under the explanation at once, its answer once it comes.
func (w *words) ask(question string) {
	question = strings.TrimSpace(question)
	e := w.explain.GetUntracked()
	if !e.open || e.busy || e.failed || question == "" {
		return
	}
	var asked []sync.ExplainQA
	for _, q := range e.asked {
		if !q.busy && !q.failed {
			asked = append(asked, sync.ExplainQA{Question: q.question, Answer: q.answer})
		}
	}
	explained := e.answer.Sections
	if len(explained) == 0 && e.answer.Text != "" {
		explained = []sync.ExplainSection{{Kind: sync.ExplainMeaning, Text: e.answer.Text}}
	}
	n := len(e.asked)
	e.asked = append(slices.Clone(e.asked), followUp{n: n, question: question, busy: true})
	w.explain.Set(e)
	gen := w.gen
	lines, at := w.p.around(e.line)
	w.p.noteStudy()
	w.p.s.Ask(w.p.key, lines, at, explained, asked, question, func(a sync.ExplainAnswer, err error) {
		e := w.explain.GetUntracked()
		if gen != w.gen || !e.open || n >= len(e.asked) {
			return
		}
		e.asked = slices.Clone(e.asked)
		q := &e.asked[n]
		q.busy = false
		if err != nil {
			q.failed, q.problem = true, session.StudyProblemOf(err)
		} else {
			q.answer = strings.TrimSpace(a.Text)
		}
		w.explain.Set(e)
	})
}

// meaning is what the explanation says the line means, "" while none came: its meaning section, or the text of a computer that sends no sections.
func (e explanation) meaning() string {
	if e.busy || e.failed {
		return ""
	}
	for _, sec := range e.answer.Sections {
		if sec.Kind == sync.ExplainMeaning {
			return strings.TrimSpace(sec.Text)
		}
	}
	if len(e.answer.Sections) == 0 {
		return strings.TrimSpace(e.answer.Text)
	}
	return ""
}

// saveExplained puts the line explained in the lines saved, which the daily review asks, with what the explanation says it means as its note; a line saved already keeps its note, or takes the explanation's when it has none.
func (w *words) saveExplained() {
	e := w.explain.GetUntracked()
	p := w.p
	if !e.open || e.line < 0 || e.line >= len(p.list()) {
		return
	}
	if !p.s.Studyable(p.key) {
		p.say("save", locales.PlayerCannotSave(p.cx))
		return
	}
	cue, note := p.studyCue(e.line), e.meaning()
	if line, ok := p.s.SavedAt(p.key, cue.Start); ok {
		if note != "" && strings.TrimSpace(line.Note) == "" {
			p.s.SetNote(p.key, cue, note)
		}
		return
	}
	p.s.SetNote(p.key, cue, note)
}

// picked is the word looked up in line i, "" for none. Read it in a binding.
func (w *words) picked(i int) string {
	if l := w.lookup.Get(); l.open && l.line == i {
		return l.word
	}
	return ""
}

// tappable is t, the text of line i, with each of its words a span a tap looks up, and the line's word marks drawn on it (markSpans) in the colors of tone.
func (w *words) tappable(t core.Text, i int, text string, tone func(string) (core.Color, bool)) core.Text {
	ink := markInk{style: t.Style, look: func(word string) { w.look(i, word) }, tone: tone}
	t.Spans, t.Content = nil, ""
	t.SpansFunc = func() []core.Span {
		ink.picked = w.picked(i)
		return markSpans(text, w.p.tokens(text), w.p.lineMarks(i), ink)
	}
	return t
}

// tokens cuts text, a line of the film, into its words, its Japanese and Chinese by dictionary (segment.Words). The dictionary is read in the background the first time a line needs it; read in a binding, the line is cut again once it is.
func (p *play) tokens(text string) []kcore.WordToken {
	p.cut.Get()
	return segment.Words(text, p.cutAgain)
}

// cutAgain has the lines cut again (tokens), once a dictionary is read: segment.Words calls it, on the goroutine that read the dictionary, once for each line cut while it was being read, and those calls make one post.
func (p *play) cutAgain() {
	if p.recut.Swap(true) {
		return
	}
	p.cx.Post(func() {
		p.recut.Store(false)
		p.cut.Set(p.cut.GetUntracked() + 1)
	})
}

// computer is the name of the film's computer, as the sheets say who answers.
func computer(p *play) string {
	f, _ := p.s.LinesFilm(p.key)
	if d, ok := p.s.Desktop(f.Desktop); ok {
		return d.Name
	}
	return ""
}

// problemText is why the computer could not answer, as the sheets say it.
func problemText(cx *core.Scope, p *play, problem session.StudyProblem) string {
	name := computer(p)
	switch problem {
	case session.StudyNoAI:
		return locales.PlayerStudyNoAI(cx, name)
	case session.StudyUnpaired:
		return locales.PlayerStudyUnpaired(cx)
	case session.StudyFailed:
		return locales.PlayerStudyFailed(cx, name)
	}
	return locales.PlayerStudyUnreachable(cx, name)
}

// sourceText is where the meaning of a word looked up came from, as the computer's own card says it: a dictionary installed on the computer, with the credit its license asks for, a lookup source of a plugin on the computer, by its name and its plugin's, or the AI service of the computer that answered, by the name it gave with the answer, which the phone kept with the word, rather than the computer of the film open now, which need not be the one that answered; "" for a computer that does not say, or a source this phone does not know.
func sourceText(cx *core.Scope, src sync.WordSource) string {
	switch src.Kind {
	case sync.SourceDictionary:
		if src.Name == "" {
			return ""
		}
		if src.Credit != "" {
			return locales.PlayerWordSourceDictCredit(cx, src.Name, src.Credit)
		}
		return locales.PlayerWordSourceDict(cx, src.Name)
	case sync.SourcePlugin:
		if src.Name == "" || src.Plugin == "" {
			return ""
		}
		return locales.PlayerWordSourcePlugin(cx, src.Plugin, src.Name)
	case sync.SourceAI:
		if src.Desktop == "" {
			return locales.PlayerWordSourceAIUnnamed(cx)
		}
		return locales.PlayerWordSourceAI(cx, src.Desktop)
	}
	return ""
}

// wordStatus is what shows of the word looked up while the computer is asked, or why it could not answer.
func wordStatus(cx *core.Scope, p *play, w *words) []core.Node {
	colors := ui.ColorsOf(cx)
	l := func() lookup { return w.lookup.Get() }
	return []core.Node{
		core.Show{When: func() bool { return l().busy }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				ui.Spinner{Size: 16},
				core.Text{TestID: "player-word-busy", ContentFunc: func() string { return locales.PlayerAsking(cx, computer(p)) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			}}
		}},
		core.Show{When: func() bool { return l().failed }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "player-word-problem", ContentFunc: func() string { return problemText(cx, p, l().problem) }, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
		}},
	}
}

// answered reports whether the computer told the meaning of the word looked up. Read it in a binding.
func (w *words) answered() bool {
	l := w.lookup.Get()
	return !l.busy && !l.failed && l.entry.Meaning != ""
}

// wordSource is where the meaning of the word looked up came from (sourceText), "" while none shows.
func wordSource(cx *core.Scope, w *words) func() string {
	return func() string {
		if !w.answered() {
			return ""
		}
		return sourceText(cx, w.lookup.Get().entry.Source)
	}
}

// lineWithWord is line's text in spans, word in it marked as the design's highlighter does, bold on yellow; the whole line plain when word is not one of its words.
func lineWithWord(p *play, text, word string) []core.Span {
	var spans []core.Span
	plain := ""
	for _, t := range p.tokens(text) {
		if t.Word && word != "" && strings.EqualFold(t.Text, word) {
			if plain != "" {
				spans = append(spans, core.Span{Content: plain})
				plain = ""
			}
			spans = append(spans, core.Span{Content: t.Text, Style: core.Style{FontWeight: look.Heavy, Background: theme.SubtitleYellow.Faded(0.45), CornerRadius: 3}})
			continue
		}
		plain += t.Text
	}
	if plain != "" {
		spans = append(spans, core.Span{Content: plain})
	}
	return spans
}

// wordSheet is the word looked up in a sheet over the player, upright and on a tablet, in the app's appearance: the word as a dictionary lists it, its reading and a button that says it, its meaning on a card, marked AI's where an AI service gave it, where it came from, the line it was tapped in with the word marked, the film and how many times the film says it; then the buttons that play the line again and put the word in the vocabulary, or take it out. The sheet closes by its grabber, a tap beside it or Back.
// The design's part of speech before the meaning, and the word's base form, are left out: sync.WordEntry has neither.
func wordSheet(cx *core.Scope, p *play, w *words, when func() bool) core.Node {
	return appSheet{Open: func() bool { return w.lookup.Get().open && when() }, OnClose: w.closeLookup, Label: func() string { return w.lookup.Get().word },
		Children: []core.Node{core.Func(func(cx *core.Scope) core.Node { return wordSheetBody(cx, p, w) })}}
}

func wordSheetBody(cx *core.Scope, p *play, w *words) core.Node {
	colors := ui.ColorsOf(cx)
	l := func() lookup { return w.lookup.Get() }
	card := sheetCard(colors)
	f, _ := p.s.LinesFilm(p.key)
	count := func() int {
		x := l()
		cut := make([][]kcore.WordToken, 0, len(p.list()))
		for _, c := range p.list() {
			cut = append(cut, p.tokens(c.Original))
		}
		return timesSaid(cut, x.word, x.entry.Word)
	}
	speak := func() {
		x := l()
		_ = cx.Speak(cmp.Or(x.entry.Word, x.word), core.Voice{Lang: f.Film.Spoken}, nil)
	}
	poster := func() core.ImageSource { k, _ := p.s.Film(p.key); return look.LocalPoster(p.s, k) }
	children := []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{TestID: "player-word", ContentFunc: func() string { return cmp.Or(l().entry.Word, l().word) }, Role: core.RoleHeader, MaxLines: 1,
					Style: core.Style{FontSize: 30, LineHeight: 36, FontWeight: look.Heavy, LetterSpacing: -0.4}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return l().reading() != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-word-reading", ContentFunc: func() string { return l().reading() }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
				}},
			}},
			core.Show{When: func() bool { return cx.Env().CanSpeak() }, Then: func(cx *core.Scope) core.Node {
				return base.Button{TestID: "player-word-speak", LabelFunc: func() string { return locales.PlayerWordSpeak(cx) }, OnPress: func(core.PressEvent) { speak() },
					Style:     core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 22, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
					StyleFunc: func() core.Style { return core.Style{Background: colors.Accent().Faded(0.1)} },
					Children:  []core.Node{ui.IconOf(speakerFilled, 20, colors.Accent)}}
			}},
		}},
	}
	children = append(children, wordStatus(cx, p, w)...)
	children = append(children,
		core.Show{When: w.answered, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.All(14)}, StyleFunc: card, Children: []core.Node{
				core.Show{When: func() bool { return l().entry.Source.Kind == sync.SourceAI }, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Height: core.Pt(22), Padding: core.Symmetric(0, 7), CornerRadius: 6, JustifyContent: core.JustifyCenter},
						StyleFunc: func() core.Style { return core.Style{Background: colors.Accent().Faded(0.1)} },
						Children:  []core.Node{core.Text{ContentFunc: func() string { return locales.PlayerWordAIBadge(cx) }, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Accent)}}}
				}},
				core.Text{TestID: "player-word-meaning", ContentFunc: func() string { return l().entry.Meaning }, LiveRegion: core.LiveRegionPolite,
					Style: core.Style{FontSize: 18, LineHeight: 24, FontWeight: core.FontWeightBold, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
			}}
		}},
		core.Show{When: func() bool { return wordSource(cx, w)() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "player-word-source", ContentFunc: wordSource(cx, w), Style: look.Footnote.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)}
		}},
		core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.PlayerWordInLine(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
			core.View{Style: core.Style{Gap: 6, Padding: core.All(14)}, StyleFunc: card, Children: []core.Node{
				core.Text{TestID: "player-word-line", Style: core.Style{FontSize: 16, LineHeight: 23}, StyleFunc: ui.Colored(colors.Text),
					SpansFunc: func() []core.Span { x := l(); return lineWithWord(p, p.line(x.line).Original, x.word) }},
				core.Show{When: func() bool { return p.line(l().line).Translation != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{ContentFunc: func() string { return p.line(l().line).Translation }, Style: core.Style{FontSize: 14, LineHeight: 20}, StyleFunc: ui.Colored(colors.Muted)}
				}},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Margin: core.Edges{Top: 4}}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(18), CornerRadius: 3, Overflow: core.Hidden}, StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} },
						Children: []core.Node{core.Image{SourceFunc: poster, ResizeMode: core.Cover, ElementsHidden: true, Style: fill}}},
					core.Text{Content: f.Film.Title, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
					core.Text{TestID: "player-word-times", ContentFunc: func() string { return locales.PlayerWordTimes(cx, count()) }, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
				}},
			}},
		}},
		// Replay takes its words' width and the vocabulary the rest: its English (In Vocabulary · Remove) is too long for the design's 197.
		core.View{Style: core.Style{Direction: core.Row, Gap: 10, Margin: core.Edges{Top: 4}}, Children: []core.Node{
			base.Button{TestID: "player-word-replay", LabelFunc: func() string { return locales.PlayerReplayLine(cx) },
				OnPress: func(core.PressEvent) {
					i := w.lookup.GetUntracked().line
					w.closeLookup()
					p.step(i)
				},
				Style: core.Style{FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pt(50), Padding: core.Symmetric(0, 20), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Active: &core.Style{Opacity: core.Alpha(0.7)}},
				StyleFunc: func() core.Style { return core.Style{Background: colors.Surface(), Color: colors.Accent()} },
				Children:  []core.Node{core.Text{ContentFunc: func() string { return locales.PlayerReplayLine(cx) }, MaxLines: 1, Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold}}}},
			vocabButton(cx, p, w, 50, 14, 16),
		}},
	)
	return core.View{TestID: "player-word-sheet", Style: core.Style{Gap: 12, Padding: core.Edges{Bottom: 8}}, Children: children}
}

// wordPopover is the word looked up in a light card at the corner of the picture, on a phone on its side, where a sheet would cover the line: the word, its reading and ×, its meaning, and the button that puts it in the vocabulary or takes it out.
func wordPopover(cx *core.Scope, p *play, w *words, when func() bool) core.Node {
	return core.Show{When: func() bool { return w.lookup.Get().open && when() }, Then: func(cx *core.Scope) core.Node {
		cx.SetSubtreeAppearance(core.AppearanceLight)
		colors := ui.ColorsOf(cx)
		l := func() lookup { return w.lookup.Get() }
		children := []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{
				core.Text{TestID: "player-word", ContentFunc: func() string { return cmp.Or(l().entry.Word, l().word) }, Role: core.RoleHeader, MaxLines: 1,
					Style: core.Style{FontSize: 20, LineHeight: 25, FontWeight: look.Heavy, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return l().reading() != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-word-reading", ContentFunc: func() string { return l().reading() }, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 17, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Muted)}
				}},
				core.View{Style: core.Style{FlexGrow: 1}},
				ui.IconButton{TestID: "player-word-close", Icon: lucide.X, LabelFunc: func() string { return locales.PlayerClose(cx) }, Variant: ui.IconButtonGhost, Size: 28, OnPress: w.closeLookup},
			}},
		}
		children = append(children, wordStatus(cx, p, w)...)
		children = append(children,
			core.Show{When: w.answered, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "player-word-meaning", ContentFunc: func() string { return l().entry.Meaning }, LiveRegion: core.LiveRegionPolite, MaxLines: 4,
					Style: core.Style{FontSize: 14, LineHeight: 19}, StyleFunc: ui.Colored(colors.Text)}
			}},
			core.Show{When: func() bool { return wordSource(cx, w)() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "player-word-source", ContentFunc: wordSource(cx, w), MaxLines: 2, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)}
			}},
			core.View{Style: core.Style{Margin: core.Edges{Top: 2}}, Children: []core.Node{vocabButton(cx, p, w, 36, 10, 14)}},
		)
		return core.View{TestID: "player-word-popover", Role: core.RoleDialog, LabelFunc: func() string { return w.lookup.Get().word },
			Style: core.Style{Position: core.Absolute, Top: core.Pt(64), Left: core.Pt(16), Width: core.Pt(260), ZIndex: 4, Gap: 8, Padding: core.Symmetric(14, 16),
				CornerRadius: 18, BorderCurve: core.BorderCurveContinuous, PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea},
				Shadows: []core.BoxShadow{{Offset: core.Point{Y: 16}, Blur: 40, Color: core.RGBA(0x000000, 0.35)}}},
			StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Surface(), 0.96)} },
			Children:  children}
	}}
}

// breakdownItem is a word of an explanation's breakdown, as the computer writes it on a line of its own: the words and what they mean.
type breakdownItem struct{ word, meaning string }

// breakdownItems are the words of a breakdown section written a line each as "word · meaning", nil when a line of it is written otherwise, which then shows as it is.
func breakdownItems(text string) []breakdownItem {
	var out []breakdownItem
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		word, meaning, ok := strings.Cut(line, " · ")
		if !ok || strings.TrimSpace(word) == "" || strings.TrimSpace(meaning) == "" {
			return nil
		}
		out = append(out, breakdownItem{strings.TrimSpace(word), strings.TrimSpace(meaning)})
	}
	return out
}

// explainSheet is the line explained, in a sheet over the player in the app's appearance: the line with its translation, then what the computer's AI service said of it in sections, each under its heading, the breakdown's words as rows that look each up, with More for its tone and related expressions, which model said it and where it ran, or that it is asked, or why it could not answer; the follow-up questions asked and the field that asks one, Save Line to Notes, and the button that fixes the line's translation (fixSheet).
func explainSheet(cx *core.Scope, p *play, w *words, fx *fixer) core.Node {
	e := func() explanation { return w.explain.Get() }
	return appSheet{Open: func() bool { return e().open }, OnClose: w.closeExplain, Label: func() string { return locales.PlayerExplainTitle(cx) },
		Children: []core.Node{core.Func(func(cx *core.Scope) core.Node { return explainBody(cx, p, w, fx) })}}
}

func explainBody(cx *core.Scope, p *play, w *words, fx *fixer) core.Node {
	colors := ui.ColorsOf(cx)
	e := func() explanation { return w.explain.Get() }
	ai := func() core.Color { return theme.AI }
	ready := func() bool {
		x := e()
		return !x.busy && !x.failed && (len(x.answer.Sections) > 0 || x.answer.Text != "")
	}
	by := func() string {
		x := e().answer
		if x.Local {
			return locales.PlayerExplainBy(cx, x.Model, computer(p))
		}
		return locales.PlayerExplainVia(cx, x.Model, computer(p))
	}
	// The sections shown: those before More, then the rest once More is pressed. A computer that sends no sections has its text shown as it is.
	shown := core.NewMemoFunc(cx, func() []sync.ExplainSection {
		x := e()
		var out []sync.ExplainSection
		for _, sec := range x.answer.Sections {
			if strings.TrimSpace(sec.Text) != "" && (x.more || !explainKept(sec.Kind)) {
				out = append(out, sec)
			}
		}
		return out
	}, slices.Equal[[]sync.ExplainSection])
	hasMore := func() bool {
		x := e()
		return !x.more && slices.ContainsFunc(x.answer.Sections, func(sec sync.ExplainSection) bool { return explainKept(sec.Kind) && strings.TrimSpace(sec.Text) != "" })
	}
	cardStyle := sheetCard(colors)
	card := func(children ...core.Node) core.View {
		return core.View{Style: core.Style{Padding: core.All(14), Gap: 4}, StyleFunc: cardStyle, Children: children}
	}
	// lookUp looks a word of the breakdown up in the line explained, the sheet giving way to the word's.
	lookUp := func(word string) {
		i := w.explain.GetUntracked().line
		w.closeExplain()
		w.look(i, word)
	}
	sections := core.Each(shown.Get, func(sec sync.ExplainSection) string { return sec.Kind }, func(cx *core.Scope, sec func() sync.ExplainSection, _ func() int) core.Node {
		items := func() []breakdownItem {
			if sec().Kind != sync.ExplainBreakdown {
				return nil
			}
			return breakdownItems(sec().Text)
		}
		return core.Show{When: func() bool { return len(items()) > 0 },
			Then: func(cx *core.Scope) core.Node {
				rows := core.Each(items, func(it breakdownItem) string { return it.word }, func(cx *core.Scope, it func() breakdownItem, i func() int) core.Node {
					return base.Button{TestID: "player-explain-word", LabelFunc: func() string { return it().word + ", " + it().meaning }, OnPress: func(core.PressEvent) { lookUp(it().word) },
						Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(46), Padding: core.Symmetric(6, 16)},
						StyleFunc: func() core.Style {
							st := core.Style{Active: &core.Style{Background: colors.Pressed()}}
							if i() > 0 {
								st.Borders, st.BorderColors = core.Edges{Top: 0.5}, core.EdgeColors{Top: colors.Line()}
							}
							return st
						},
						Children: []core.Node{
							core.Text{ContentFunc: func() string { return it().word }, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
							core.Text{ContentFunc: func() string { return it().meaning }, MaxLines: 2, Style: core.Style{FontSize: 14, LineHeight: 19, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Muted)},
							ui.IconOf(look.Chevron, 13, func() core.Color { return theme.Chevron }),
						}}
				})
				return core.View{TestID: "player-explain-section", Style: core.Style{Overflow: core.Hidden}, StyleFunc: cardStyle, Children: []core.Node{rows}}
			},
			Else: func(cx *core.Scope) core.Node {
				return card(
					core.Show{When: func() bool { return explainHeading(cx, sec().Kind) != "" }, Then: func(cx *core.Scope) core.Node {
						return core.Text{ContentFunc: func() string { return explainHeading(cx, sec().Kind) }, Role: core.RoleHeader, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(ai)}
					}},
					core.Text{TestID: "player-explain-section", ContentFunc: func() string { return sec().Text }, Style: core.Style{FontSize: 15, LineHeight: 22}, StyleFunc: ui.Colored(colors.Text)},
				)
			}}
	})
	question := core.NewSignal(cx, "")
	send := func() {
		w.ask(question.GetUntracked())
		question.Set("")
	}
	asked := func() []followUp { return e().asked }
	followUps := core.Each(asked, func(q followUp) int { return q.n }, func(cx *core.Scope, q func() followUp, _ func() int) core.Node {
		return card(
			core.Text{ContentFunc: func() string { return q().question }, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Text)},
			core.Text{TestID: "player-explain-answer", LiveRegion: core.LiveRegionPolite, Style: look.Subhead,
				ContentFunc: func() string {
					switch x := q(); {
					case x.busy:
						return locales.PlayerExplainAnswering(cx)
					case x.failed:
						return problemText(cx, p, x.problem)
					default:
						return x.answer
					}
				},
				StyleFunc: func() core.Style {
					switch x := q(); {
					case x.failed:
						return core.Style{Color: colors.Danger()}
					case x.busy:
						return core.Style{Color: colors.Muted()}
					}
					return core.Style{Color: colors.Text()}
				}},
		)
	})
	saved := func() bool { return p.isSaved(e().line) }
	saveText := func() string {
		if saved() {
			return locales.PlayerExplainSaved(cx)
		}
		return locales.PlayerExplainSave(cx)
	}
	skeleton := func(width float32) core.Node {
		return core.View{Style: core.Style{Height: core.Pt(14), Width: core.Pct(width), CornerRadius: 7, Animation: core.KeyframeAnimation{Keyframes: core.KeyframesPulse}},
			StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} }}
	}
	return core.Scroll{TestID: "player-explain", Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.View{Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 8}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(34), Height: core.Pt(34), CornerRadius: 10, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Gradients: []core.Gradient{{Kind: core.GradientLinear, Angle: 135, Stops: []core.ColorStop{{Color: theme.AI}, {Color: theme.PlayerBlue}}}}},
					Children: []core.Node{ui.IconOf(sparkleFilled, 18, func() core.Color { return theme.PlayerInk })}},
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return locales.PlayerExplainTitle(cx) }, Role: core.RoleHeader, Style: look.Title.Merge(core.Style{FontWeight: look.Heavy}), StyleFunc: ui.Colored(colors.Text)},
					core.Show{When: func() bool { return e().answer.Model != "" }, Then: func(cx *core.Scope) core.Node {
						return core.Text{TestID: "player-explain-by", ContentFunc: by, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)}
					}},
				}},
			}},
			card(
				core.Text{TestID: "player-explain-line", ContentFunc: func() string { return p.line(e().line).Original }, Style: core.Style{FontSize: 17, LineHeight: 24, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return p.line(e().line).Translation != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{ContentFunc: func() string { return p.line(e().line).Translation }, Style: core.Style{FontSize: 14, LineHeight: 20}, StyleFunc: ui.Colored(colors.Muted)}
				}},
			),
			core.Show{When: func() bool { return e().busy }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 10, Padding: core.Symmetric(4, 4)}, Children: []core.Node{
					core.Text{TestID: "player-explain-busy", ContentFunc: func() string { return locales.PlayerAsking(cx, computer(p)) }, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
					skeleton(80), skeleton(95), skeleton(60),
				}}
			}},
			core.Show{When: func() bool { return e().failed }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "player-explain-problem", ContentFunc: func() string { return problemText(cx, p, e().problem) }, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
			}},
			core.Show{When: func() bool { x := e(); return ready() && len(x.answer.Sections) == 0 }, Then: func(cx *core.Scope) core.Node {
				return card(core.Text{TestID: "player-explain-text", ContentFunc: func() string { return e().answer.Text }, LiveRegion: core.LiveRegionPolite, Style: core.Style{FontSize: 15, LineHeight: 22}, StyleFunc: ui.Colored(colors.Text)})
			}},
			core.Show{When: ready, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
					core.View{TestID: "player-explain-sections", Style: core.Style{Gap: 12}, Children: []core.Node{sections}},
					core.Show{When: hasMore, Then: func(cx *core.Scope) core.Node {
						return core.View{Style: core.Style{AlignItems: core.AlignStart}, Children: []core.Node{
							look.Pill(cx, "player-explain-more", func() string { return locales.PlayerExplainMore(cx) }, w.expandExplain),
						}}
					}},
					core.Text{TestID: "player-explain-note", ContentFunc: func() string { return locales.PlayerExplainOnlyBefore(cx) }, Style: look.Footnote.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
					core.View{TestID: "player-explain-asked", Style: core.Style{Gap: 8}, Children: []core.Node{followUps}},
					core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
						ui.Input{TestID: "player-explain-ask", LabelFunc: func() string { return locales.PlayerExplainAskLabel(cx) }, Value: question.Get, OnChange: question.Set, OnSubmit: func(string) { send() },
							PlaceholderFunc: func() string { return locales.PlayerExplainAskPlaceholder(cx) }, ReturnKeyType: core.ReturnKeySend,
							Style: core.Style{FlexGrow: 1, FlexShrink: 1, Height: core.Pt(44), FontSize: 16, CornerRadius: 12, Padding: core.Symmetric(0, 12)}},
						look.Pill(cx, "player-explain-send", func() string { return locales.PlayerExplainAsk(cx) }, send),
					}},
					look.Button{TestID: "player-explain-save", Kind: look.Tinted, Icon: lucide.Star, Text: saveText, OnPress: w.saveExplained},
				}}
			}},
			core.Show{When: func() bool { return fx.canFix(e().line) }, Then: func(cx *core.Scope) core.Node {
				return base.Button{TestID: "player-explain-fix", LabelFunc: func() string { return locales.PlayerFixOpen(cx) },
					OnPress: func(core.PressEvent) {
						i := w.explain.GetUntracked().line
						w.closeExplain()
						fx.start(i)
					},
					Style:     core.Style{Height: core.Pt(46), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
					StyleFunc: func() core.Style { return core.Style{Background: colors.Surface(), Color: colors.Accent()} },
					Children:  []core.Node{core.Text{ContentFunc: func() string { return locales.PlayerFixOpen(cx) }, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}}}}
			}},
		}},
	}}
}

// explainKept reports whether the sections of kind wait for More: the tone and related expressions, as the computer's side panel keeps them.
func explainKept(kind string) bool { return kind == sync.ExplainTone || kind == sync.ExplainRelated }

// explainHeading is the heading of a section of kind, in the language of the interface; "" for a kind this phone does not know, shown under no heading.
func explainHeading(cx *core.Scope, kind string) string {
	switch kind {
	case sync.ExplainMeaning:
		return locales.PlayerExplainMeaning(cx)
	case sync.ExplainBreakdown:
		return locales.PlayerExplainBreakdown(cx)
	case sync.ExplainTranslation:
		return locales.PlayerExplainTranslation(cx)
	case sync.ExplainTone:
		return locales.PlayerExplainTone(cx)
	case sync.ExplainRelated:
		return locales.PlayerExplainRelated(cx)
	}
	return ""
}
