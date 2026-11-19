package player

import (
	"slices"
	"strings"
	"time"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/study"
	kcore "kando/packages/core"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Fixing a line's translation on the phone (改译文): the explained line's sheet offers it (译文不对？改一下), and a sheet over the player takes the learner's translation, which the computer keeps as a correction of the line (session.Correct) and shows wherever the line is read. With 同时加进术语表 on, the words the fix changed go into the film's glossary as well, through the computer (session.AddTerm), so translating the film again writes them the learner's way: the words as the original line says them, which the learner types, and their translation, filled in with what the fix wrote; a term the glossary has whose translation the fix replaced is filled in whole.

// fixing is the line being fixed: its index, the translation typed, and, with term on, the term added with it, source as the original says it and target its translation; typedTarget is set once the learner typed the term's translation, which then no longer follows the fix.
type fixing struct {
	open           bool
	line           int
	draft          string
	term           bool
	source, target string
	typedTarget    bool
}

// fixer is the fix sheet's state.
type fixer struct {
	p     *play
	state *core.Signal[fixing]
}

func newFixer(cx *core.Scope, p *play) *fixer {
	return &fixer{p: p, state: core.NewSignal(cx, fixing{})}
}

// canFix reports whether line i's translation can be fixed: the film has one, and the computer takes the phone's fixes (session.CanCorrect). Read it in a binding.
func (f *fixer) canFix(i int) bool {
	p := f.p
	return !p.untranslated && i >= 0 && i < len(p.list()) && p.s.CanCorrect(p.key)
}

// start opens the sheet on line i, its translation in the field: the film holds while it shows.
func (f *fixer) start(i int) {
	if !f.canFix(i) {
		return
	}
	if f.p.playing.GetUntracked() {
		f.p.toggle()
	}
	f.state.Set(fixing{open: true, line: i, draft: f.p.line(i).Translation})
}

// close closes the sheet, dropping what was typed.
func (f *fixer) close() { f.state.Set(fixing{}) }

// edit keeps draft as the translation typed, and the term's translation following the words it changed while the learner has not typed one.
func (f *fixer) edit(draft string) {
	x := f.state.GetUntracked()
	x.draft = draft
	if x.term && !x.typedTarget {
		x.source, x.target = f.suggest(x.line, draft, x.source)
	}
	f.state.Set(x)
}

// setTerm turns adding a term with the fix on or off, filling the term in from the fix when it is turned on.
func (f *fixer) setTerm(on bool) {
	x := f.state.GetUntracked()
	x.term = on
	if on && !x.typedTarget {
		x.source, x.target = f.suggest(x.line, x.draft, x.source)
	}
	f.state.Set(x)
}

// suggest is the term a fix of line i to draft fills in, source as typed so far: the words the fix wrote (kcore.FixOf), and the glossary's term whose translation the fix replaced in a line that holds its words, whose words then are the source.
func (f *fixer) suggest(i int, draft, source string) (string, string) {
	p := f.p
	line := p.line(i)
	fix, ok := kcore.FixOf(line.Translation, strings.TrimSpace(draft))
	if !ok {
		return source, ""
	}
	if g, ok := p.s.Glossary(p.key); ok {
		for _, t := range g.Terms {
			if t.Target == fix.From && kcore.TermIn(line.Original, t.Source) {
				return t.Source, fix.To
			}
		}
	}
	return source, fix.To
}

// ready reports whether the sheet can save: a translation is typed, other than the line's, and with the term on and offered, the term's words and translation, the words found in the original line.
func (f *fixer) ready() bool {
	x := f.state.Get()
	line := f.p.line(x.line)
	draft := strings.TrimSpace(x.draft)
	if draft == "" || draft == line.Translation {
		return false
	}
	// The term's fields count only while they show: a computer that stops taking terms meanwhile (read-only, or no glossary) leaves the fix alone to save, as save does.
	if !x.term || !f.p.s.CanAddTerm(f.p.key) {
		return true
	}
	return strings.TrimSpace(x.target) != "" && kcore.TermIn(line.Original, x.source)
}

// save keeps the fix, and adds the term with it when the switch is on, and says so.
func (f *fixer) save() {
	if !f.ready() {
		return
	}
	p, x := f.p, f.state.GetUntracked()
	if !p.s.Correct(p.key, p.studyCue(x.line), x.draft) {
		p.say("fix", locales.PlayerFixCannot(p.cx))
		return
	}
	f.close()
	if !x.term || !p.s.CanAddTerm(p.key) {
		p.say("fix", locales.PlayerFixSaved(p.cx))
		return
	}
	term := sync.Term{Source: strings.TrimSpace(x.source), Target: strings.TrimSpace(x.target), Locked: true}
	p.s.AddTerm(p.key, term, func(err error) {
		if err != nil {
			p.say("fix", locales.PlayerFixTermFailed(p.cx))
			return
		}
		p.say("fix", locales.PlayerFixSavedTerm(p.cx))
	})
}

// corrected is list with the fixes laid over the lines they fix, found by where the lines show; a fix of no line the list has is left out, as the computer's next answer brings it.
func corrected(list []cues.Cue, fixes []study.Correction) []cues.Cue {
	if len(fixes) == 0 {
		return list
	}
	out := slices.Clone(list)
	for _, c := range fixes {
		i := slices.IndexFunc(out, func(l cues.Cue) bool { return near(l.Start, c.Start) && near(l.End, c.End) })
		if i < 0 {
			continue
		}
		if c.Original != nil {
			out[i].Original = *c.Original
		}
		if c.Translation != nil {
			out[i].Translation = *c.Translation
		}
	}
	return out
}

// near reports whether two times are one line's, as the learning state takes them (study.SameStart).
func near(a, b time.Duration) bool { d := a - b; return d < study.SameStart && d > -study.SameStart }

// fixSheet is the fix of a line's translation, in a sheet over the player: Cancel, the title and Save, the original line, the field the translation is typed in, the switch that adds a term with the fix and, while it is on, the term's fields.
func fixSheet(cx *core.Scope, p *play, f *fixer) core.Node {
	colors := ui.ColorsOf(cx)
	x := func() fixing { return f.state.Get() }
	field := func(id string, label, placeholder func() string, value func() string, set func(string), multiline bool) core.Node {
		st := core.Style{FontSize: 16, CornerRadius: 12, Padding: core.Symmetric(0, 12), Height: core.Pt(44)}
		if multiline {
			st = core.Style{FontSize: 17, CornerRadius: 12, MinHeight: core.Pt(88)}
		}
		return ui.Input{TestID: id, LabelFunc: label, PlaceholderFunc: placeholder, Value: value, OnChange: set, Multiline: multiline, Style: st}
	}
	card := func(children ...core.Node) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, Gap: 6}, StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.06)} },
			Children: children}
	}
	// The bar's buttons are its text alone, in the accent, as a sheet's Cancel and Save are; Save is bold and dims until there is something to save.
	barButton := func(id string, text func() string, bold bool, disabled func() bool, press func()) core.Node {
		weight := core.FontWeightNormal
		if bold {
			weight = core.FontWeightSemibold
		}
		return base.Button{TestID: id, LabelFunc: text, Disabled: disabled, OnPress: func(core.PressEvent) { press() },
			Style:    core.Style{MinHeight: core.Pt(44), MinWidth: core.Pt(44), JustifyContent: core.JustifyCenter, Disabled: &core.Style{Opacity: core.Alpha(0.35)}},
			Children: []core.Node{core.Text{ContentFunc: text, PointerEvents: core.PointerEventsNone, MaxLines: 1, Style: look.Body.Merge(core.Style{FontWeight: weight}), StyleFunc: ui.Colored(colors.Accent)}}}
	}
	caption := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}
	}
	termFields := core.Show{When: func() bool { return x().term }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "player-fix-term", Style: core.Style{Gap: 8}, Children: []core.Node{
			field("player-fix-term-source", func() string { return locales.PlayerFixTermSource(cx) }, func() string { return locales.PlayerFixTermSourceHint(cx) },
				func() string { return x().source }, func(v string) { s := f.state.GetUntracked(); s.source = v; f.state.Set(s) }, false),
			field("player-fix-term-target", func() string { return locales.PlayerFixTermTarget(cx) }, func() string { return locales.PlayerFixTermTargetHint(cx) },
				func() string { return x().target }, func(v string) { s := f.state.GetUntracked(); s.target, s.typedTarget = v, true; f.state.Set(s) }, false),
		}}
	}}
	return ui.Sheet{Open: func() bool { return x().open }, OnClose: f.close, Children: []core.Node{
		core.Scroll{TestID: "player-fix", Style: core.Style{MaxHeight: core.Pct(85)}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 8}}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
					barButton("player-fix-cancel", func() string { return locales.PlayerFixCancel(cx) }, false, nil, f.close),
					core.Text{ContentFunc: func() string { return locales.PlayerFixTitle(cx) }, Role: core.RoleHeader, Style: look.Title.Merge(core.Style{FlexGrow: 1, TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
					barButton("player-fix-save", func() string { return locales.PlayerFixSave(cx) }, true, func() bool { return !f.ready() }, f.save),
				}},
				card(
					caption(func() string { return locales.PlayerFixOriginal(cx) }),
					core.Text{TestID: "player-fix-original", ContentFunc: func() string { return p.line(x().line).Original }, Style: look.Body, StyleFunc: ui.Colored(colors.Text)},
				),
				core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
					caption(func() string { return locales.PlayerFixYours(cx) }),
					field("player-fix-draft", func() string { return locales.PlayerFixYours(cx) }, nil, func() string { return x().draft }, f.edit, true),
				}},
				core.Show{When: func() bool { return p.s.CanAddTerm(p.key) }, Then: func(cx *core.Scope) core.Node {
					return card(
						core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(44)}, Children: []core.Node{
							core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
								core.Text{ContentFunc: func() string { return locales.PlayerFixAddTerm(cx) }, Style: look.Body, StyleFunc: ui.Colored(colors.Text)},
								caption(func() string { return locales.PlayerFixAddTermDetail(cx) }),
							}},
							// TODO(nsgo#145): ui.Switch's Label is a string only (ui/switch.go:19), so its spoken name keeps the language it was built in; pass a LabelFunc once Switch has one https://github.com/nsgo-dev/nsgo/issues/145
							ui.Switch{TestID: "player-fix-term-switch", Label: locales.PlayerFixAddTerm(cx), Variant: ui.SwitchSystem, Checked: func() bool { return x().term }, OnChange: f.setTerm},
						}},
						termFields,
					)
				}},
				caption(func() string { return locales.PlayerFixNote(cx) }),
			}},
		}},
	}}
}
