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

// tappable is t, the text of line i, with each of its words a span a tap looks up.
func (w *words) tappable(t core.Text, i int, text string) core.Text {
	t.Spans, t.Content = nil, ""
	t.SpansFunc = func() []core.Span {
		var spans []core.Span
		for _, tok := range w.p.tokens(text) {
			sp := core.Span{Content: tok.Text}
			if tok.Word {
				word := tok.Text
				sp.OnPress = func(core.PressEvent) { w.look(i, word) }
			}
			spans = append(spans, sp)
		}
		return spans
	}
	return t
}

// tokens cuts text, a line of the film, into its words, its Japanese and Chinese by dictionary (segment.Words). The dictionary is read in the background the first time a line needs it; read in a binding, the line is cut again once it is.
func (p *play) tokens(text string) []kcore.WordToken {
	p.cut.Get()
	return segment.Words(text, func() { p.cx.Post(func() { p.cut.Set(p.cut.GetUntracked() + 1) }) })
}

// computer is the name of the film's computer, as the sheets say who answers.
func computer(p *play) string {
	f, _ := p.s.Film(p.key)
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

// wordBody is the card of the word looked up: the word as a dictionary lists it, its reading and its meaning, or that the computer is asked, or why it could not answer; the line it was tapped in, with its translation.
func wordBody(cx *core.Scope, p *play, w *words, compact bool) []core.Node {
	colors := ui.ColorsOf(cx)
	l := func() lookup { return w.lookup.Get() }
	wordSize := core.Points(30)
	if compact {
		wordSize = 22
	}
	return []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{TestID: "player-word", ContentFunc: func() string { return cmp.Or(l().entry.Word, l().word) }, Role: core.RoleHeader, MaxLines: 1,
					Style: core.Style{FontSize: wordSize, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return l().entry.Reading != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-word-reading", ContentFunc: func() string { return l().entry.Reading }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
				}},
			}},
			core.Show{When: func() bool { return compact }, Then: func(cx *core.Scope) core.Node { return vocabStar(cx, p, w) }},
			ui.IconButton{TestID: "player-word-close", Icon: lucide.X, LabelFunc: func() string { return locales.PlayerClose(cx) }, Variant: ui.IconButtonGhost, Size: 36, OnPress: w.closeLookup},
		}},
		core.Show{When: func() bool { return l().busy }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				ui.Spinner{Size: 16},
				core.Text{TestID: "player-word-busy", ContentFunc: func() string { return locales.PlayerAsking(cx, computer(p)) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			}}
		}},
		core.Show{When: func() bool { return l().failed }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "player-word-problem", ContentFunc: func() string { return problemText(cx, p, l().problem) }, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
		}},
		core.Show{When: func() bool { return !l().busy && !l().failed && l().entry.Meaning != "" }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous}, StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.06)} },
				Children: []core.Node{core.Text{TestID: "player-word-meaning", ContentFunc: func() string { return l().entry.Meaning }, LiveRegion: core.LiveRegionPolite, Style: look.Body, StyleFunc: ui.Colored(colors.Text)}}}
		}},
		core.Show{When: func() bool { return !compact }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.PlayerWordInLine(cx) }, Style: look.Section, StyleFunc: ui.Colored(colors.Muted)},
				core.Text{TestID: "player-word-line", ContentFunc: func() string { return p.line(l().line).Original }, Style: look.Body, StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: func() string { return p.line(l().line).Translation }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			}}
		}},
	}
}

// wordSheet is the word looked up in a sheet over the player, upright and on a tablet, with the buttons that put it in the vocabulary, or take it out, and play its line again.
func wordSheet(cx *core.Scope, p *play, w *words, when func() bool) core.Node {
	body := wordBody(cx, p, w, false)
	body = append(body, vocabButton(cx, p, w), look.Button{TestID: "player-word-replay", Kind: look.Tinted, Icon: lucide.RotateCcw, Text: func() string { return locales.PlayerReplayLine(cx) },
		OnPress: func() {
			i := w.lookup.GetUntracked().line
			w.closeLookup()
			p.step(i)
		}})
	return ui.Sheet{Open: func() bool { return w.lookup.Get().open && when() }, OnClose: w.closeLookup, Children: []core.Node{
		core.View{TestID: "player-word-sheet", Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 8}}, Children: body},
	}}
}

// wordPopover is the word looked up in a card at the corner of the picture, on a phone on its side, where a sheet would cover the line, with a star that puts it in the vocabulary or takes it out.
func wordPopover(cx *core.Scope, p *play, w *words, when func() bool) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return w.lookup.Get().open && when() }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "player-word-popover", Role: core.RoleDialog, LabelFunc: func() string { return w.lookup.Get().word },
			Style: core.Style{Position: core.Absolute, Top: core.Pt(56), Left: core.Pt(16), Width: core.Pt(280), ZIndex: 4, Gap: 10, Padding: core.All(14),
				CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea}},
			StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} },
			Children:  wordBody(cx, p, w, true)}
	}}
}

// explainSheet is the line explained, in a sheet over the player: the line with its translation, then what the computer's AI service said of it in sections, each under its heading, with More for its tone and related expressions, which model said it and where it ran, or that it is asked, or why it could not answer; the follow-up questions asked and the field that asks one, and Save Line to Notes.
func explainSheet(cx *core.Scope, p *play, w *words) core.Node {
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
	card := func(children ...core.Node) core.View {
		return core.View{Style: core.Style{Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, Gap: 4}, StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.06)} },
			Children: children}
	}
	sections := core.Each(shown.Get, func(sec sync.ExplainSection) string { return sec.Kind }, func(cx *core.Scope, sec func() sync.ExplainSection, _ func() int) core.Node {
		return card(
			core.Show{When: func() bool { return explainHeading(cx, sec().Kind) != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: func() string { return explainHeading(cx, sec().Kind) }, Role: core.RoleHeader, Style: look.Section, StyleFunc: ui.Colored(ai)}
			}},
			core.Text{TestID: "player-explain-section", ContentFunc: func() string { return sec().Text }, Style: look.Body, StyleFunc: ui.Colored(colors.Text)},
		)
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
	return ui.Sheet{Open: func() bool { return e().open }, OnClose: w.closeExplain, Children: []core.Node{
		core.Scroll{TestID: "player-explain", Style: core.Style{MaxHeight: core.Pct(80)}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 8}}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 9, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: theme.AI.Faded(0.18)},
						Children: []core.Node{ui.IconOf(lucide.Sparkles, 18, ai)}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return locales.PlayerExplainTitle(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
						core.Show{When: func() bool { return e().answer.Model != "" }, Then: func(cx *core.Scope) core.Node {
							return core.Text{TestID: "player-explain-by", ContentFunc: by, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}
						}},
					}},
					ui.IconButton{TestID: "player-explain-close", Icon: lucide.X, LabelFunc: func() string { return locales.PlayerClose(cx) }, Variant: ui.IconButtonGhost, Size: 36, OnPress: w.closeExplain},
				}},
				card(
					core.Text{TestID: "player-explain-line", ContentFunc: func() string { return p.line(e().line).Original }, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightMedium}), StyleFunc: ui.Colored(colors.Text)},
					core.Text{ContentFunc: func() string { return p.line(e().line).Translation }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
				),
				core.Show{When: func() bool { return e().busy }, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
						core.Text{TestID: "player-explain-busy", ContentFunc: func() string { return locales.PlayerAsking(cx, computer(p)) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
						ui.Skeleton{Lines: 3},
					}}
				}},
				core.Show{When: func() bool { return e().failed }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-explain-problem", ContentFunc: func() string { return problemText(cx, p, e().problem) }, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
				}},
				core.Show{When: func() bool { x := e(); return ready() && len(x.answer.Sections) == 0 }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-explain-text", ContentFunc: func() string { return e().answer.Text }, LiveRegion: core.LiveRegionPolite, Style: look.Body, StyleFunc: ui.Colored(colors.Text)}
				}},
				core.Show{When: ready, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Gap: 10}, Children: []core.Node{
						core.View{TestID: "player-explain-sections", Style: core.Style{Gap: 10}, Children: []core.Node{sections}},
						core.Show{When: hasMore, Then: func(cx *core.Scope) core.Node {
							return core.View{Style: core.Style{AlignItems: core.AlignStart}, Children: []core.Node{
								look.Pill(cx, "player-explain-more", func() string { return locales.PlayerExplainMore(cx) }, w.expandExplain),
							}}
						}},
						core.Text{TestID: "player-explain-note", ContentFunc: func() string { return locales.PlayerExplainOnlyBefore(cx) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Faint)},
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
