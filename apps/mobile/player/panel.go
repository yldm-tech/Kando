package player

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/study"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
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
	selected := core.NewSignal(cx, linesTab)
	// The tab bar is drawn on base.Tabs: ui.Tabs' underline is the accent's and as wide as its tab (ui/tabs.go:97-102), where the design's is white and as wide as the tab's text, and its tabs take no count beside their name.
	// TODO(nsgo#1037): ui.Tabs takes no colour or width for its underline (ui/tabs.go:120-124); draw the panel's tabs with ui.Tabs, their counts in each option's Trailing, once it does. https://github.com/nsgo-dev/nsgo/issues/1037
	item := func(t tab, id string, text func() string, count func() int) core.Node {
		on := func() bool { return selected.Get() == t }
		// The tab is the bar's 38 tall whatever its count: its name 14 high from 10 down, and its underline 3 tall at its foot (the design draws it at 36, over the hairline, which the scroll clips), as the design's.
		return base.Tab{TestID: id, Selected: on, OnPress: func(core.PressEvent) { selected.Set(t) },
			Style: core.Style{Direction: core.Column, Height: core.Pt(38), MinHeight: core.Pt(38), MaxHeight: core.Pt(38), Padding: core.Edges{Top: 10}},
			Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 3, Height: core.Pt(25)}, Children: []core.Node{
					core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 17}, StyleFunc: func() core.Style {
						if on() {
							return core.Style{FontWeight: core.FontWeightBold, Color: theme.PlayerInk}
						}
						return core.Style{FontWeight: core.FontWeightMedium, Color: theme.PlayerInk.Faded(0.55)}
					}},
					core.Show{When: func() bool { return count() > 0 }, Then: func(cx *core.Scope) core.Node {
						return core.Text{ContentFunc: func() string { return strconv.Itoa(count()) }, MaxLines: 1, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightMedium, Color: theme.PlayerInk.Faded(0.5)}}
					}},
				}},
				core.View{Style: core.Style{Height: core.Pt(3), MinHeight: core.Pt(3), AlignSelf: core.AlignStretch, CornerRadius: 2}, StyleFunc: func() core.Style {
					if on() {
						return core.Style{Background: theme.PlayerInk}
					}
					return core.Style{}
				}},
			}}
	}
	none := func() int { return 0 }
	// TODO(nsgo#145): base.Tabs' Label is a string only, so the tab list's spoken name keeps the language it was built in; pass a LabelFunc once Tabs has one https://github.com/nsgo-dev/nsgo/issues/145
	bar := base.Tabs{TestID: "player-tabs", Label: locales.PlayerPanel(cx), Style: core.Style{Direction: core.Row, Gap: 20, Height: core.Pt(38)}, Children: []core.Node{
		item(linesTab, "player-tab-lines", func() string { return locales.PlayerTabLines(cx) }, none),
		item(wordsTab, "player-tab-words", func() string { return locales.PlayerTabWords(cx) }, func() int { return len(p.filmWords()) }),
		item(notesTab, "player-tab-notes", func() string { return locales.PlayerTabNotes(cx) }, func() int { return len(p.s.Saved(p.key)) }),
		item(partsTab, "player-tab-parts", func() string { return locales.PlayerTabParts(cx) }, none),
		item(infoTab, "player-tab-info", func() string { return locales.PlayerTabInfo(cx) }, none),
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
		// TODO(nsgo#967): a Scroll shrinks to fit its parent (core/tree.go:2083) and a Style cannot say FlexShrink 0, its zero being unset (core/style.go:35, Merge), so the bar's height is held by MinHeight; drop it once a Scroll's shrink can be turned off. https://github.com/nsgo-dev/nsgo/issues/967
		core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{Height: core.Pt(38.5), MinHeight: core.Pt(38.5), Margin: core.Edges{Top: 10}, Borders: core.Edges{Bottom: 0.5}, BorderColors: core.EdgeColors{Bottom: theme.PlayerInk.Faded(0.14)}},
			Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(38), Padding: core.Symmetric(0, 20)}, Children: []core.Node{bar}}}},
		page(linesTab, func(cx *core.Scope) core.Node { return lines(cx, p, false) }),
		page(wordsTab, func(cx *core.Scope) core.Node { return wordsPage(cx, p, w) }),
		page(notesTab, func(cx *core.Scope) core.Node { return notesPage(cx, p) }),
		page(partsTab, func(cx *core.Scope) core.Node { return partsPage(cx, p) }),
		page(infoTab, func(cx *core.Scope) core.Node { return infoPage(cx, p) }),
	}}
}

// faint is the white of the panel's lesser text at alpha.
func faint(alpha float32) core.Color { return theme.PlayerInk.Faded(alpha) }

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
	noteFocused := core.NewSignal(cx, false)
	heading := func() string {
		i := target()
		return locales.PlayerNoteHeading(cx, i+1, len(p.list()), clock(p.line(i).Start))
	}
	return core.Scroll{TestID: "player-notes", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 12, Right: 12, Bottom: 16, Left: 12}, Gap: 16}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 10, Padding: core.All(12), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, Background: faint(0.08)}, Children: []core.Node{
				core.Text{ContentFunc: heading, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, Color: faint(0.55)}},
				core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
					ui.Input{TestID: "player-note", LabelFunc: func() string { return locales.PlayerNoteFor(cx, target()+1) }, Value: draft.Get, OnChange: write, OnSubmit: func(string) { save() },
						PlaceholderFunc: func() string { return locales.PlayerNotePlaceholder(cx) }, ReturnKeyType: core.ReturnKeyDone, Disabled: func() bool { return target() < 0 },
						OnFocus: func() { noteFocused.Set(true) }, OnBlur: func() { noteFocused.Set(false) },
						Style:     core.Style{FlexGrow: 1, FlexShrink: 1, Height: core.Pt(38), FontSize: 14, CornerRadius: 10, Padding: core.Symmetric(0, 12)},
						StyleFunc: func() core.Style { return rimless(faint(0.1), noteFocused.Get(), theme.PlayerBlue) }},
					base.Button{TestID: "player-note-save", LabelFunc: func() string { return locales.PlayerNoteSave(cx) }, OnPress: func(core.PressEvent) { save() },
						Style: core.Style{Height: core.Pt(38), Padding: core.Symmetric(0, 16), CornerRadius: 10, JustifyContent: core.JustifyCenter, Background: theme.PlayerBlue,
							Active: &core.Style{Opacity: core.Alpha(0.7)}},
						Children: []core.Node{core.Text{ContentFunc: func() string { return locales.PlayerNoteSave(cx) }, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightBold, Color: theme.PlayerInk}}}},
				}},
			}},
			core.Show{When: func() bool { return len(saved()) == 0 }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "player-notes-empty", ContentFunc: func() string { return locales.PlayerNotesEmpty(cx) }, Style: core.Style{FontSize: 13, LineHeight: 18, Padding: core.Symmetric(0, 4), Color: faint(0.55)}}
			}},
			core.Each(saved, func(l study.Line) string { return l.ID }, func(cx *core.Scope, l func() study.Line, _ func() int) core.Node {
				return core.View{TestID: "player-saved-line", Role: core.RoleButton, LabelFunc: func() string { return l().Original },
					OnPress: func(core.PressEvent) {
						if i := cues.Index(p.list(), l().Start); i >= 0 {
							p.step(i)
						}
					},
					Style:     core.Style{Gap: 4, Padding: core.Symmetric(4, 4), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
					StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} },
					Children: []core.Node{
						core.Text{ContentFunc: func() string { return clock(l().Start) }, Style: mono.Merge(core.Style{FontSize: 11, LineHeight: 14, Color: faint(0.5)})},
						core.Text{ContentFunc: func() string { return l().Original }, Style: core.Style{FontSize: 15, LineHeight: 21, FontWeight: core.FontWeightSemibold, Color: theme.PlayerInk}},
						core.Text{ContentFunc: func() string { return l().Translation }, Style: core.Style{FontSize: 13, LineHeight: 18, Color: faint(0.6)}},
						core.Show{When: func() bool { return l().Note != "" }, Then: func(cx *core.Scope) core.Node {
							return core.View{Style: core.Style{Margin: core.Edges{Top: 4}, Padding: core.Symmetric(8, 10), CornerRadius: 10, Background: theme.SubtitleYellow.Faded(0.14)}, Children: []core.Node{
								core.Text{TestID: "player-saved-note", ContentFunc: func() string { return l().Note }, Style: core.Style{FontSize: 13, LineHeight: 18, Color: theme.NoteInk}},
							}}
						}},
					}}
			}),
		}},
	}}
}

// part is a part of the film's lines: its number, from 1, when it starts and ends, and its first line, its translation where it has one.
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
		first := list[i].Translation
		if strings.TrimSpace(first) == "" {
			first = list[i].Original
		}
		out = append(out, part{n: n + 1, start: from, end: to, first: first})
	}
	return out
}

// partsPage is the film in equal parts, the one playing marked, each with the film's poster, as the phone has no picture of each part, and each of which a tap plays from.
func partsPage(cx *core.Scope, p *play) core.Node {
	colors := ui.ColorsOf(cx)
	all := func() []part { return parts(p.list(), p.length.Get()) }
	poster := func() core.ImageSource {
		f, _ := p.s.Film(p.key)
		return look.LocalPoster(p.s, f)
	}
	return core.Scroll{TestID: "player-parts", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 12, Right: 10, Bottom: 16, Left: 10}, Gap: 4}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.PlayerPartsHint(cx) }, Style: core.Style{FontSize: 12, LineHeight: 16, Padding: core.Edges{Left: 10, Right: 10, Bottom: 6}, Color: faint(0.55)}},
			core.Each(all, func(pt part) int { return pt.n }, func(cx *core.Scope, pt func() part, _ func() int) core.Node {
				playing := func() bool { at := p.at.Get(); return at >= pt().start && at < pt().end }
				return core.View{TestID: "player-part", Role: core.RoleButton, LabelFunc: func() string { return locales.PlayerPart(cx, pt().n, clock(pt().start), clock(pt().end)) },
					Selected: playing,
					OnPress: func(core.PressEvent) {
						if i := cues.Next(p.list(), pt().start-1); i >= 0 {
							p.step(i)
						}
					},
					Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(8, 10), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
					StyleFunc: func() core.Style {
						st := core.Style{Active: &core.Style{Background: colors.Pressed()}}
						if playing() {
							st.Background = faint(0.13)
						}
						return st
					},
					Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(84), Height: core.Pt(47), CornerRadius: 7, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, FlexShrink: 0, Background: faint(0.08)},
							Children: []core.Node{core.Image{SourceFunc: poster, ResizeMode: core.Cover, ElementsHidden: true, Style: fill}}},
						core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
							core.Text{ContentFunc: func() string { return locales.PlayerPart(cx, pt().n, clock(pt().start), clock(pt().end)) }, MaxLines: 1,
								Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightSemibold, Color: theme.PlayerInk}},
							core.Text{ContentFunc: func() string { return pt().first }, MaxLines: 2, Style: core.Style{FontSize: 14, LineHeight: 19, Color: theme.PlayerInk}},
						}},
					}}
			}),
		}},
	}}
}

// sourceName is where a film comes from, as the films list's chips name it.
func sourceName(cx *core.Scope, source string) string {
	switch source {
	case catalog.SourceYouTube:
		return locales.FilmsSourceYoutube(cx)
	case catalog.SourceBilibili:
		return locales.FilmsSourceBilibili(cx)
	case catalog.SourceLocal:
		return locales.FilmsSourceLocal(cx)
	}
	return locales.FilmsSourceWeb(cx)
}

// infoPage is what the film is, on a card: where it comes from, its length with the number of its lines, the language spoken and the one it is translated into, its computer and its library; then its glossary (glossaryList), and the button that shares its subtitles as a file.
// The design's rows of how the film was transcribed and translated (the model, the AI service) and of its subtitle files are left out: sync.Film does not say them.
func infoPage(cx *core.Scope, p *play) core.Node {
	film := func() (sync.Film, string) {
		f, _ := p.s.Film(p.key)
		return f.Film, f.Library
	}
	first := true
	row := func(id string, name, value func() string) core.Node {
		top := !first
		first = false
		return core.View{TestID: id, Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.Symmetric(12, 0)}, StyleFunc: func() core.Style {
			if top {
				return core.Style{Borders: core.Edges{Top: 0.5}, BorderColors: core.EdgeColors{Top: faint(0.1)}}
			}
			return core.Style{}
		},
			Children: []core.Node{
				core.Text{ContentFunc: name, Style: core.Style{FontSize: 14, LineHeight: 20, Width: core.Pt(96), FlexShrink: 0, Color: faint(0.55)}},
				core.Text{ContentFunc: value, Style: core.Style{FontSize: 14, LineHeight: 20, FlexGrow: 1, FlexShrink: 1, Color: theme.PlayerInk}},
			}}
	}
	when := func(on func() bool, node core.Node) core.Node {
		return core.Show{When: on, Then: func(*core.Scope) core.Node { return node }}
	}
	spoken := func() string { f, _ := film(); return look.Languages(sync.Film{Spoken: f.Spoken}) }
	translated := func() string { f, _ := film(); return f.Translation }
	rows := []core.Node{
		row("player-info-source", func() string { return locales.PlayerInfoSource(cx) }, func() string { f, _ := film(); return sourceName(cx, f.Source) }),
		row("player-info-length", func() string { return locales.PlayerInfoLength(cx) }, func() string {
			return locales.PlayerInfoLengthValue(cx, clock(p.length.Get()), len(p.list()))
		}),
	}
	rows = append(rows,
		when(func() bool { return spoken() != "" }, row("player-info-audio", func() string { return locales.PlayerInfoAudio(cx) }, spoken)),
		when(func() bool { return translated() != "" }, row("player-info-translation", func() string { return locales.PlayerInfoTranslation(cx) }, translated)),
		row("player-info-computer", func() string { return locales.PlayerInfoComputer(cx) }, func() string { return computer(p) }),
		row("player-info-library", func() string { return locales.PlayerInfoLibrary(cx) }, func() string { _, l := film(); return l }),
	)
	return core.Scroll{TestID: "player-info", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 12, Right: 12, Bottom: 16, Left: 12}, Gap: 16}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.Symmetric(0, 14), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, Background: faint(0.08)}, Children: rows},
			glossaryList(cx, p),
			base.Button{TestID: "player-export", LabelFunc: func() string { return locales.PlayerExportSubtitles(cx) }, OnPress: func(core.PressEvent) { exportSubtitles(cx, p) },
				Style: core.Style{Height: core.Pt(42), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Background: faint(0.12),
					Active: &core.Style{Opacity: core.Alpha(0.7)}},
				Children: []core.Node{core.Text{ContentFunc: func() string { return locales.PlayerExportSubtitles(cx) }, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightBold, Color: theme.PlayerInk}}}},
		}},
	}}
}

// exportSubtitles has the film's computer write its subtitles as a file and shows the share sheet for it (session.ExportSubtitles, which says itself why it could not).
func exportSubtitles(cx *core.Scope, p *play) {
	f, _ := p.s.Film(p.key)
	p.s.ExportSubtitles(f.Desktop, f.Film, func(path string, err error) {
		if err == nil {
			cx.Share(core.ShareOptions{Files: []core.SharedFile{{Path: path}}}, nil)
		}
	})
}

// glossaryList is the film's glossary as its computer translates it (prototype template 1003-1004): the terms as the lines say them and as they are translated, a lock on those the translation must write as they are, which the phone shows and the computer changes, or that the film has none yet. Nothing shows until the computer answered, nor for one that does not share it.
func glossaryList(cx *core.Scope, p *play) core.Node {
	terms := core.NewMemoFunc(cx, func() []sync.Term { g, _ := p.s.Glossary(p.key); return g.Terms }, slices.Equal[[]sync.Term]).Get
	return core.Show{When: func() bool { _, ok := p.s.Glossary(p.key); return ok }, Then: func(cx *core.Scope) core.Node {
		rows := core.Each(terms, func(t sync.Term) string { return strings.ToLower(t.Source) }, func(cx *core.Scope, t func() sync.Term, i func() int) core.Node {
			return core.View{TestID: "player-term", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(11, 0)},
				StyleFunc: func() core.Style {
					if i() > 0 {
						return core.Style{Borders: core.Edges{Top: 0.5}, BorderColors: core.EdgeColors{Top: faint(0.1)}}
					}
					return core.Style{}
				},
				Children: []core.Node{
					core.Text{TestID: "player-term-source", ContentFunc: func() string { return t().Source }, Style: core.Style{FontSize: 14, LineHeight: 19, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), Color: theme.PlayerInk}},
					core.Text{TestID: "player-term-target", ContentFunc: func() string { return t().Target }, Style: core.Style{FontSize: 14, LineHeight: 19, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), Color: faint(0.7)}},
					core.Show{When: func() bool { return t().Locked }, Then: func(cx *core.Scope) core.Node {
						return core.View{TestID: "player-term-locked", Role: core.RoleImage, LabelFunc: func() string { return locales.PlayerTermLocked(cx) }, Children: []core.Node{ui.IconOf(lucide.Lock, 13, func() core.Color { return faint(0.55) })}}
					}},
				}}
		})
		return core.View{TestID: "player-terms", Role: core.RoleGroup, LabelFunc: func() string { return locales.PlayerTerms(cx) }, Style: core.Style{Gap: 8}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8, Padding: core.Symmetric(0, 4)}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.PlayerTerms(cx) }, Role: core.RoleHeader, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightSemibold, FlexGrow: 1, Color: faint(0.55)}},
				core.Text{ContentFunc: func() string { return locales.PlayerTermsOnComputer(cx) }, Style: core.Style{FontSize: 12, LineHeight: 16, Color: faint(0.45)}},
			}},
			core.View{Style: core.Style{Padding: core.Symmetric(0, 14), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, Background: faint(0.08)}, Children: []core.Node{
				rows,
				core.Show{When: func() bool { return len(terms()) == 0 }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-terms-empty", ContentFunc: func() string { return locales.PlayerTermsEmpty(cx) }, Style: core.Style{FontSize: 14, LineHeight: 19, Padding: core.Symmetric(11, 0), Color: faint(0.55)}}
				}},
			}},
		}}
	}}
}
