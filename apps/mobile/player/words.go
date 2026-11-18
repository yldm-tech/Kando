package player

import (
	"cmp"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"
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

// explanation is a line explained: the line, busy while the computer is asked, then what it answered, or why it could not.
type explanation struct {
	open    bool
	line    int
	busy    bool
	answer  sync.Explanation
	failed  bool
	problem session.StudyProblem
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
	return &words{cx: cx, p: p, lookup: core.NewSignal(cx, lookup{}), explain: core.NewSignal(cx, explanation{})}
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

// closeExplain closes the line explained, dropping the answer on its way.
func (w *words) closeExplain() {
	w.gen++
	w.explain.Set(explanation{})
}

// tappable is t, the text of line i, with each of its words a span a tap looks up.
func (w *words) tappable(t core.Text, i int, text string) core.Text {
	t.Spans = nil
	for _, tok := range kcore.WordTokens(text) {
		sp := core.Span{Content: tok.Text}
		if tok.Word {
			word := tok.Text
			sp.OnPress = func(core.PressEvent) { w.look(i, word) }
		}
		t.Spans = append(t.Spans, sp)
	}
	t.Content = ""
	return t
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

// wordSheet is the word looked up in a sheet over the player, upright and on a tablet, with the button that plays its line again.
func wordSheet(cx *core.Scope, p *play, w *words, when func() bool) core.Node {
	body := wordBody(cx, p, w, false)
	body = append(body, look.Button{TestID: "player-word-replay", Kind: look.Tinted, Icon: lucide.RotateCcw, Text: func() string { return locales.PlayerReplayLine(cx) },
		OnPress: func() {
			i := w.lookup.GetUntracked().line
			w.closeLookup()
			p.step(i)
		}})
	return ui.Sheet{Open: func() bool { return w.lookup.Get().open && when() }, OnClose: w.closeLookup, Children: []core.Node{
		core.View{TestID: "player-word-sheet", Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 8}}, Children: body},
	}}
}

// wordPopover is the word looked up in a card at the corner of the picture, on a phone on its side, where a sheet would cover the line.
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

// explainSheet is the line explained, in a sheet over the player: the line with its translation, then what the computer's AI service said of it, which model on which computer, or that it is asked, or why it could not answer.
func explainSheet(cx *core.Scope, p *play, w *words) core.Node {
	colors := ui.ColorsOf(cx)
	e := func() explanation { return w.explain.Get() }
	ai := func() core.Color { return theme.AI }
	return ui.Sheet{Open: func() bool { return e().open }, OnClose: w.closeExplain, Children: []core.Node{
		core.Scroll{TestID: "player-explain", Style: core.Style{MaxHeight: core.Pct(80)}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 8}}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 9, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: theme.AI.Faded(0.18)},
						Children: []core.Node{ui.IconOf(lucide.Sparkles, 18, ai)}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return locales.PlayerExplainTitle(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
						core.Show{When: func() bool { return e().answer.Model != "" }, Then: func(cx *core.Scope) core.Node {
							return core.Text{TestID: "player-explain-by", ContentFunc: func() string { return locales.PlayerExplainBy(cx, e().answer.Model, computer(p)) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}
						}},
					}},
					ui.IconButton{TestID: "player-explain-close", Icon: lucide.X, LabelFunc: func() string { return locales.PlayerClose(cx) }, Variant: ui.IconButtonGhost, Size: 36, OnPress: w.closeExplain},
				}},
				core.View{Style: core.Style{Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, Gap: 4}, StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.06)} },
					Children: []core.Node{
						core.Text{TestID: "player-explain-line", ContentFunc: func() string { return p.line(e().line).Original }, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightMedium}), StyleFunc: ui.Colored(colors.Text)},
						core.Text{ContentFunc: func() string { return p.line(e().line).Translation }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
					}},
				core.Show{When: func() bool { return e().busy }, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
						core.Text{TestID: "player-explain-busy", ContentFunc: func() string { return locales.PlayerAsking(cx, computer(p)) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
						ui.Skeleton{Lines: 3},
					}}
				}},
				core.Show{When: func() bool { return e().failed }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-explain-problem", ContentFunc: func() string { return problemText(cx, p, e().problem) }, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
				}},
				core.Show{When: func() bool { return !e().busy && !e().failed && e().answer.Text != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-explain-text", ContentFunc: func() string { return e().answer.Text }, LiveRegion: core.LiveRegionPolite, Style: look.Body, StyleFunc: ui.Colored(colors.Text)}
				}},
			}},
		}},
	}}
}
