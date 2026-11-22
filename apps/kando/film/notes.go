package film

import (
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// SideNotes is the FilmSide of the 笔记 tab.
const SideNotes = "notes"

// Notebook is what the 笔记 tab reads of the open film's saved lines and words and the review, and the clicks it reports.
type Notebook interface {
	// Notes is the open film's saved lines and the words of the vocabulary looked up in it, each by where its line starts. NotesWords is whether the tab lists the words rather than the lines.
	Notes() []Note
	NotesWords() *bool
	// RemoveNote takes a line or a word out of the study record.
	RemoveNote(n Note)
	// DueCount is how many saved lines and words of the library are due for review now, and StartReview reviews them.
	DueCount() int
	StartReview()
}

// Note is a saved line of the open film, or with Word set a word of the vocabulary looked up in it: its id in the study record, where its line starts, the line's Original and Translation, and the learner's Note on a line, or a word's Text, Reading and Meaning.
type Note struct {
	ID                    string
	Word                  bool
	Start                 time.Duration
	Original, Translation string
	Note                  string
	Text, Reading         string
	Meaning               string
}

// notesTab is the 笔记 tab (prototype template 1044-1059): the card that starts today's review with how many are due, the choice of the lines or the words, each of the open film's listed by where it is, a click playing from there and its × taking it out of the notes, and once there are lines, that each keeps its sound.
func (s *screen) notesTab(cx *ns.Scope) ns.Node {
	a := s.a
	notes := live.Func(a, a.Notes)
	words := live.Func(a, func() bool { return *a.NotesWords() })
	count := func(word bool) int {
		n := 0
		for _, note := range notes() {
			if note.Word == word {
				n++
			}
		}
		return n
	}
	shown := func() []Note {
		w := words()
		var out []Note
		for _, note := range notes() {
			if note.Word == w {
				out = append(out, note)
			}
		}
		return out
	}
	rows := ns.Each(shown, func(n Note) string { return n.ID }, func(cx *ns.Scope, note func() Note, index func() int) ns.Node {
		return s.noteRow(cx, note, index)
	})
	list := ns.Show{
		When: func() bool { return len(shown()) > 0 },
		Then: func(*ns.Scope) ns.Node {
			return ns.View{TestID: "film-notes-list", Style: ns.Style{CornerRadius: 14, Background: panelCard}, Children: []ns.Node{rows}}
		},
		Else: func(cx *ns.Scope) ns.Node {
			return ns.View{Style: ns.Style{CornerRadius: 14, Background: panelCard}, Children: []ns.Node{
				ns.Text{TestID: "film-notes-empty", ContentFunc: func() string {
					if words() {
						return locales.FilmNotesEmptyWords(cx)
					}
					return locales.FilmNotesEmptyLines(cx)
				}, Style: ns.Style{FontSize: 12, LineHeight: 18, Color: onGlass, Opacity: ns.Alpha(0.65), Padding: ns.Symmetric(14, 12)}},
			}}
		},
	}
	segment := func(word bool, id string, label func() string) ns.Node {
		on := func() bool { return words() == word }
		return base.Tab{TestID: id, Selected: on,
			OnPress: live.On(a, func(ns.PressEvent) { *a.NotesWords() = word }),
			Style: ns.Style{FlexGrow: 1, FlexBasis: ns.Pt(0), Height: ns.Pt(24), CornerRadius: 8, JustifyContent: ns.JustifyCenter, AlignItems: ns.AlignCenter,
				Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
			StyleFunc: func() ns.Style {
				if on() {
					return ns.Style{Background: ns.RGBA(0xFFFFFF, 0.22)}
				}
				return ns.Style{}
			},
			Children: []ns.Node{ns.Text{ContentFunc: label, MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass},
				StyleFunc: func() ns.Style {
					if on() {
						return ns.Style{FontWeight: ns.FontWeightSemibold}
					}
					return ns.Style{FontWeight: ns.FontWeightNormal}
				}}}}
	}
	segments := base.Tabs{TestID: "film-notes-kind", LabelFunc: func() string { return locales.FilmNotesKind(cx) },
		Style: ns.Style{Direction: ns.Row, Padding: ns.All(3), CornerRadius: 10, Background: ns.RGBA(0xFFFFFF, 0.08)},
		Children: []ns.Node{
			segment(false, "film-notes-lines", func() string { return locales.FilmNotesLines(cx, count(false)) }),
			segment(true, "film-notes-words", func() string { return locales.FilmNotesWords(cx, count(true)) }),
		}}
	kept := ns.Show{When: func() bool { return !words() && count(false) > 0 }, Then: func(cx *ns.Scope) ns.Node {
		return ns.Text{TestID: "film-notes-sound", ContentFunc: func() string { return locales.FilmNotesSoundKept(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.55)}}
	}}
	return ns.Scroll{TestID: "film-notes", Style: ns.Style{FlexGrow: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
		ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 14, Left: 14}, Gap: 10}, Children: []ns.Node{
			s.reviewCard(cx), segments, list, kept,
		}},
	}}
}

// reviewCard is the 笔记 tab's card that starts today's review: what it holds, or that it is done or has nothing yet, and how many are due.
func (s *screen) reviewCard(cx *ns.Scope) ns.Node {
	a := s.a
	due := live.Func(a, a.DueCount)
	some := live.Func(a, func() bool { return len(a.Notes()) > 0 })
	title := func() string { return locales.FilmNotesReview(cx) }
	sub := func() string {
		switch {
		case due() > 0:
			return locales.FilmNotesReviewDue(cx)
		case some():
			return locales.FilmNotesReviewDone(cx)
		}
		return locales.FilmNotesReviewEmpty(cx)
	}
	return base.Button{TestID: "film-notes-review", LabelFunc: func() string { return locales.FilmNotesReviewLabel(cx, due()) },
		Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOff},
		OnPress: live.On(a, func(ns.PressEvent) { a.StartReview() }),
		Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 10, Padding: ns.All(12), CornerRadius: 14,
			Shadows: []ns.BoxShadow{{Spread: 0.5, Color: ns.RGBA(0x8CBEFF, 0.45), Inset: true}},
			Hover:   &ns.Style{Opacity: ns.Alpha(0.9)}, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() ns.Style { return ns.Style{Background: ui.Alpha(ui.ColorsOf(cx).AccentFill(), 0.35)} },
		Children: []ns.Node{
			ns.View{PointerEvents: ns.PointerEventsNone, Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{
				ns.Text{ContentFunc: title, Style: ns.Style{FontSize: 13, LineHeight: 17, FontWeight: ns.FontWeightBold, Color: onGlass}},
				ns.Text{TestID: "film-notes-review-sub", ContentFunc: sub, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.75)}},
			}},
			ns.Text{TestID: "film-notes-due", PointerEvents: ns.PointerEventsNone, ContentFunc: func() string { return locales.FilmNotesDueCount(cx, due()) },
				Style: ns.Style{FontSize: 18, LineHeight: 22, FontWeight: ns.FontWeightBold, FontVariant: ns.FontTabularNums, Color: onGlass}},
		}}
}

// noteRow is a line or a word of the 笔记 tab: where its line starts, over the line's original and translation, with the note on it, or over the word and its meaning and the line it was looked up in; a click plays from the line, as a line of the subtitle list does, and its × takes it out of the notes.
func (s *screen) noteRow(cx *ns.Scope, note func() Note, index func() int) ns.Node {
	a := s.a
	first := func() string {
		n := note()
		if !n.Word {
			return n.Original
		}
		if n.Meaning == "" {
			return n.Text
		}
		return n.Text + "　" + n.Meaning
	}
	second := func() string {
		n := note()
		if n.Word {
			return n.Original
		}
		return n.Translation
	}
	meta := func() string {
		n := note()
		if n.Word && n.Reading != "" {
			return cueClockTenths(n.Start) + " · " + n.Reading
		}
		return cueClockTenths(n.Start)
	}
	text := func() string { return note().Note }
	removeLabel := func() string {
		if note().Word {
			return locales.FilmNotesRemoveWord(cx, note().Text)
		}
		return locales.FilmNotesRemoveLine(cx)
	}
	return ns.View{TestID: "film-note", Style: ns.Style{Direction: ns.Row, Gap: 10, Padding: ns.Symmetric(9, 12)},
		StyleFunc: func() ns.Style {
			if index() > 0 {
				return ns.Style{Borders: ns.Edges{Top: 1}, BorderColor: panelLine}
			}
			return ns.Style{Borders: ns.Edges{}}
		},
		Children: []ns.Node{
			base.Button{TestID: "film-note-play", LabelFunc: func() string { return locales.FilmNotesPlay(cx, cueClockTenths(note().Start), first()) },
				Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOff},
				OnPress: live.On(a, func(ns.PressEvent) { a.Seek(note().Start + a.Delay()) }),
				Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2, CornerRadius: 6,
					Hover: &ns.Style{Opacity: ns.Alpha(0.85)}, FocusVisible: ui.FocusRing(cx, -2)},
				Children: []ns.Node{
					ns.Text{ContentFunc: meta, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: 11, LineHeight: 14, FontVariant: ns.FontTabularNums, Color: onGlass, Opacity: ns.Alpha(0.6)}},
					ns.Text{TestID: "film-note-first", ContentFunc: first, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: 13, LineHeight: 18, Color: onGlass}},
					ns.Show{When: func() bool { return second() != "" }, Then: func(*ns.Scope) ns.Node {
						return ns.Text{ContentFunc: second, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass, Opacity: ns.Alpha(0.7)}}
					}},
					ns.Show{When: func() bool { return text() != "" }, Then: func(*ns.Scope) ns.Node {
						return ns.Text{TestID: "film-note-text", ContentFunc: text, PointerEvents: ns.PointerEventsNone,
							Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass, Margin: ns.Edges{Top: 2}, Padding: ns.Symmetric(4, 6), CornerRadius: 6, Background: ns.RGBA(0xFFFFFF, 0.08)}}
					}},
				}},
			ui.IconButton{TestID: "film-note-remove", LabelFunc: removeLabel, Size: 24, IconSize: 12,
				Icon: lucide.X, Tint: func() ns.Color { return onGlass },
				OnPress: live.Do(a, func() { a.RemoveNote(note()) })},
		}}
}
