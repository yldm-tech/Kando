package player

import (
	"slices"
	"strings"
	"time"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/study"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// tab is a tab of the player's panel.
type tab uint8

const (
	linesTab tab = iota
	wordsTab
	notesTab
	partsTab
	infoTab
)

// partCount is how many equal parts the Parts tab cuts the film's lines into.
const partCount = 4

// panel is the tabs under the card, or beside it on a tablet on its side: the film's lines, its words looked up or in the vocabulary, the notes on the lines saved, the film in equal parts, and what the film is. Each tab is built when first shown and kept while others show, so the lines keep where they were scrolled to.
func panel(cx *core.Scope, p *play, w *words) core.Node {
	colors := ui.ColorsOf(cx)
	selected := core.NewSignal(cx, linesTab)
	option := func(t tab, id string, text func() string) ui.Option[tab] {
		return ui.Option[tab]{Value: t, TestID: id, Content: core.Text{ContentFunc: text, MaxLines: 1}}
	}
	wordCount := func() string {
		if n := len(p.filmWords()); n > 0 {
			return locales.PlayerTabWordsCount(cx, n)
		}
		return locales.PlayerTabWords(cx)
	}
	notes := func() string {
		if n := len(p.s.Saved(p.key)); n > 0 {
			return locales.PlayerTabNotesCount(cx, n)
		}
		return locales.PlayerTabNotes(cx)
	}
	// TODO(nsgo#145): ui.Tabs' Label is a string only (ui/tabs.go:40), so the tab list's spoken name keeps the language it was built in; pass a LabelFunc once Tabs has one https://github.com/nsgo-dev/nsgo/issues/145
	bar := ui.Tabs[tab]{TestID: "player-tabs", Variant: ui.TabsUnderline, Equal: true, Label: locales.PlayerPanel(cx), Value: selected.Get, OnChange: selected.Set,
		Options: []ui.Option[tab]{
			option(linesTab, "player-tab-lines", func() string { return locales.PlayerTabLines(cx) }),
			option(wordsTab, "player-tab-words", wordCount),
			option(notesTab, "player-tab-notes", notes),
			option(partsTab, "player-tab-parts", func() string { return locales.PlayerTabParts(cx) }),
			option(infoTab, "player-tab-info", func() string { return locales.PlayerTabInfo(cx) }),
		}}
	page := func(t tab, build func(cx *core.Scope) core.Node) core.Node {
		seen := false
		return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
			StyleFunc: func() core.Style {
				if selected.Get() != t {
					return core.Style{Display: core.DisplayNone}
				}
				return core.Style{}
			},
			Children: []core.Node{core.Show{When: func() bool { seen = seen || selected.Get() == t; return seen }, Then: build}}}
	}
	return core.View{TestID: "player-panel", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Symmetric(0, 12), Borders: core.Edges{Bottom: 0.5}}, StyleFunc: func() core.Style {
			return core.Style{BorderColors: core.EdgeColors{Bottom: colors.Line()}}
		}, Children: []core.Node{bar}},
		page(linesTab, func(cx *core.Scope) core.Node { return lines(cx, p) }),
		page(wordsTab, func(cx *core.Scope) core.Node { return wordsPage(cx, p, w) }),
		page(notesTab, func(cx *core.Scope) core.Node { return notesPage(cx, p) }),
		page(partsTab, func(cx *core.Scope) core.Node { return partsPage(cx, p) }),
		page(infoTab, func(cx *core.Scope) core.Node { return infoPage(cx, p) }),
	}}
}

// notesPage is the note on the line the card shows, to write and save, then the lines saved from the film with their notes, each of which a tap plays.
func notesPage(cx *core.Scope, p *play) core.Node {
	colors := ui.ColorsOf(cx)
	draft := core.NewSignal(cx, "")
	// editing is the line the note being written is for, from the first key typed until it is saved, -1 while nothing is written: the film plays on meanwhile, and the note stays with the line it was begun on rather than being replaced by the next line's.
	editing := core.NewSignal(cx, -1)
	target := func() int {
		if e := editing.Get(); e >= 0 {
			return e
		}
		return p.shown.Get()
	}
	noteOf := func(i int) (study.Line, bool) {
		if i < 0 {
			return study.Line{}, false
		}
		return p.s.SavedAt(p.key, p.line(i).Start)
	}
	// While nothing is written, the field shows the note of the line the card shows, as saved; a sync landing meanwhile leaves what is written.
	cx.Effect(func() {
		i := p.shown.Get()
		if editing.Get() >= 0 {
			return
		}
		draft.Set(core.Untracked(cx, func() string { l, _ := noteOf(i); return l.Note }))
	})
	write := func(v string) {
		draft.Set(v)
		if editing.GetUntracked() < 0 && p.shown.GetUntracked() >= 0 {
			editing.Set(p.shown.GetUntracked())
		}
	}
	save := func() {
		i := editing.GetUntracked()
		if i < 0 {
			i = p.shown.GetUntracked()
		}
		if i < 0 {
			return
		}
		note := strings.TrimSpace(draft.GetUntracked())
		line, saved := noteOf(i)
		if note == "" && (!saved || line.Note == "") {
			p.say("note", locales.PlayerNoteEmpty(cx))
			return
		}
		if !p.s.Studyable(p.key) {
			p.say("note", locales.PlayerCannotSave(cx))
			return
		}
		p.s.SetNote(p.key, p.studyCue(i), note)
		editing.Set(-1)
		if note == "" {
			p.say("note", locales.PlayerNoteCleared(cx))
			return
		}
		p.say("note", locales.PlayerNoteSaved(cx))
	}
	saved := func() []study.Line { return p.s.Saved(p.key) }
	return core.Scroll{TestID: "player-notes", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.All(16), Gap: 14}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.PlayerNoteFor(cx, target()+1) }, Style: look.Section, StyleFunc: ui.Colored(colors.Muted)},
				core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
					ui.Input{TestID: "player-note", LabelFunc: func() string { return locales.PlayerNoteFor(cx, target()+1) }, Value: draft.Get, OnChange: write, OnSubmit: func(string) { save() },
						PlaceholderFunc: func() string { return locales.PlayerNotePlaceholder(cx) }, ReturnKeyType: core.ReturnKeyDone, Disabled: func() bool { return target() < 0 },
						Style: core.Style{FlexGrow: 1, FlexShrink: 1, Height: core.Pt(44), FontSize: 16, CornerRadius: 12, Padding: core.Symmetric(0, 12)}},
					look.Pill(cx, "player-note-save", func() string { return locales.PlayerNoteSave(cx) }, save),
				}},
			}},
			core.Show{When: func() bool { return len(saved()) == 0 }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "player-notes-empty", ContentFunc: func() string { return locales.PlayerNotesEmpty(cx) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
			}},
			core.Each(saved, func(l study.Line) string { return l.ID }, func(cx *core.Scope, l func() study.Line, _ func() int) core.Node {
				return core.View{TestID: "player-saved-line", Role: core.RoleButton, LabelFunc: func() string { return l().Original },
					OnPress: func(core.PressEvent) {
						if i := cues.Index(p.list(), l().Start); i >= 0 {
							p.step(i)
						}
					},
					Style: core.Style{Gap: 3, Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
					StyleFunc: func() core.Style {
						return core.Style{Background: ui.Alpha(colors.Text(), 0.05), Active: &core.Style{Background: colors.Pressed()}}
					},
					Children: []core.Node{
						ui.Tabular(core.Text{ContentFunc: func() string { return clock(l().Start) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}),
						core.Text{ContentFunc: func() string { return l().Original }, Style: look.Body, StyleFunc: ui.Colored(colors.Text)},
						core.Text{ContentFunc: func() string { return l().Translation }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
						core.Show{When: func() bool { return l().Note != "" }, Then: func(cx *core.Scope) core.Node {
							return core.Text{TestID: "player-saved-note", ContentFunc: func() string { return l().Note }, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightMedium, Color: originalColor})}
						}},
					}}
			}),
		}},
	}}
}

// part is a part of the film's lines: its number, from 1, when it starts and ends, and its first line.
type part struct {
	n          int
	start, end time.Duration
	first      string
}

// parts are the film's lines cut into partCount parts of equal time, those with lines in them.
func parts(list []cues.Cue, length time.Duration) []part {
	if len(list) == 0 {
		return nil
	}
	end := length
	if last := list[len(list)-1]; end < last.End || end < last.Start {
		end = max(last.End, last.Start)
	}
	var out []part
	for n := range partCount {
		from, to := end*time.Duration(n)/partCount, end*time.Duration(n+1)/partCount
		i := cues.Next(list, from-1)
		if i < 0 || list[i].Start >= to {
			continue
		}
		out = append(out, part{n: n + 1, start: from, end: to, first: list[i].Original})
	}
	return out
}

// partsPage is the film in equal parts, each of which a tap plays from.
func partsPage(cx *core.Scope, p *play) core.Node {
	colors := ui.ColorsOf(cx)
	all := func() []part { return parts(p.list(), p.length.Get()) }
	return core.Scroll{TestID: "player-parts", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.All(16), Gap: 10}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.PlayerPartsHint(cx) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			core.Each(all, func(pt part) int { return pt.n }, func(cx *core.Scope, pt func() part, _ func() int) core.Node {
				return core.View{TestID: "player-part", Role: core.RoleButton, LabelFunc: func() string { return locales.PlayerPart(cx, pt().n, clock(pt().start), clock(pt().end)) },
					OnPress: func(core.PressEvent) {
						if i := cues.Next(p.list(), pt().start-1); i >= 0 {
							p.step(i)
						}
					},
					Style: core.Style{Gap: 3, Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
					StyleFunc: func() core.Style {
						return core.Style{Background: ui.Alpha(colors.Text(), 0.05), Active: &core.Style{Background: colors.Pressed()}}
					},
					Children: []core.Node{
						core.Text{ContentFunc: func() string { return locales.PlayerPart(cx, pt().n, clock(pt().start), clock(pt().end)) }, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Text)},
						core.Text{ContentFunc: func() string { return pt().first }, MaxLines: 2, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
					}}
			}),
		}},
	}}
}

// infoPage is what the film is: its computer and library, its languages and its length with the number of its lines, and its glossary (glossaryList).
func infoPage(cx *core.Scope, p *play) core.Node {
	colors := ui.ColorsOf(cx)
	film := func() (title, library, languages string) {
		f, _ := p.s.Film(p.key)
		return f.Film.Title, f.Library, look.Languages(f.Film)
	}
	row := func(id string, name, value func() string) core.Node {
		return core.View{TestID: id, Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.Symmetric(10, 0), Borders: core.Edges{Bottom: 0.5}},
			StyleFunc: func() core.Style { return core.Style{BorderColors: core.EdgeColors{Bottom: colors.Line()}} },
			Children: []core.Node{
				core.Text{ContentFunc: name, Style: look.Subhead.Merge(core.Style{Width: core.Pt(96)}), StyleFunc: ui.Colored(colors.Muted)},
				core.Text{ContentFunc: value, Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
			}}
	}
	return core.Scroll{TestID: "player-info", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Symmetric(8, 16)}, Children: []core.Node{
			row("player-info-title", func() string { return locales.PlayerInfoTitle(cx) }, func() string { t, _, _ := film(); return t }),
			row("player-info-computer", func() string { return locales.PlayerInfoComputer(cx) }, func() string { return computer(p) }),
			row("player-info-library", func() string { return locales.PlayerInfoLibrary(cx) }, func() string { _, l, _ := film(); return l }),
			core.Show{When: func() bool { _, _, l := film(); return l != "" }, Then: func(cx *core.Scope) core.Node {
				return row("player-info-languages", func() string { return locales.PlayerInfoLanguages(cx) }, func() string { _, _, l := film(); return l })
			}},
			row("player-info-length", func() string { return locales.PlayerInfoLength(cx) }, func() string {
				return locales.PlayerInfoLengthValue(cx, clock(p.length.Get()), len(p.list()))
			}),
			glossaryList(cx, p),
		}},
	}}
}

// glossaryList is the film's glossary as its computer translates it (prototype template 1003-1004): the terms as the lines say them and as they are translated, a lock on those the translation must write as they are, which the phone shows and the computer changes, or that the film has none yet. Nothing shows until the computer answered, nor for one that does not share it.
func glossaryList(cx *core.Scope, p *play) core.Node {
	colors := ui.ColorsOf(cx)
	terms := core.NewMemoFunc(cx, func() []sync.Term { g, _ := p.s.Glossary(p.key); return g.Terms }, slices.Equal[[]sync.Term]).Get
	return core.Show{When: func() bool { _, ok := p.s.Glossary(p.key); return ok }, Then: func(cx *core.Scope) core.Node {
		rows := core.Each(terms, func(t sync.Term) string { return strings.ToLower(t.Source) }, func(cx *core.Scope, t func() sync.Term, _ func() int) core.Node {
			return core.View{TestID: "player-term", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(10, 0), Borders: core.Edges{Bottom: 0.5}},
				StyleFunc: func() core.Style { return core.Style{BorderColors: core.EdgeColors{Bottom: colors.Line()}} },
				Children: []core.Node{
					core.Text{TestID: "player-term-source", ContentFunc: func() string { return t().Source }, Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0)}), StyleFunc: ui.Colored(colors.Text)},
					core.Text{TestID: "player-term-target", ContentFunc: func() string { return t().Target }, Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0)}), StyleFunc: ui.Colored(colors.Muted)},
					core.Show{When: func() bool { return t().Locked }, Then: func(cx *core.Scope) core.Node {
						return core.View{TestID: "player-term-locked", Role: core.RoleImage, LabelFunc: func() string { return locales.PlayerTermLocked(cx) }, Children: []core.Node{ui.IconOf(lucide.Lock, 13, colors.Muted)}}
					}},
				}}
		})
		return core.View{TestID: "player-terms", Role: core.RoleGroup, LabelFunc: func() string { return locales.PlayerTerms(cx) }, Style: core.Style{Padding: core.Edges{Top: 20}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8, Padding: core.Edges{Bottom: 4}}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.PlayerTerms(cx) }, Role: core.RoleHeader, Style: look.Section.Merge(core.Style{FlexGrow: 1}), StyleFunc: ui.Colored(colors.Muted)},
				core.Text{ContentFunc: func() string { return locales.PlayerTermsOnComputer(cx) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Faint)},
			}},
			rows,
			core.Show{When: func() bool { return len(terms()) == 0 }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "player-terms-empty", ContentFunc: func() string { return locales.PlayerTermsEmpty(cx) }, Style: look.Subhead.Merge(core.Style{Padding: core.Symmetric(10, 0)}), StyleFunc: ui.Colored(colors.Muted)}
			}},
		}}
	}}
}
