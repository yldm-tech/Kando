// Package widgets is the phone app's widgets on the Home Screen and the Lock Screen of iPhones and iPads (yldm-tech/Kando#225), as the design's Settings › Widgets page shows them: the streak with what is due for review, and the film to go on with, the line it was left on and how far it played. WidgetKit draws them from what the app last wrote (NSGo's widget package), whether the app runs or not, so the study widget's timeline runs a few days ahead, an entry from each midnight. Android has no widgets of NSGo's yet (nsgo#657), and the app writes none there.
package widgets

import (
	"log"
	"path/filepath"
	"strconv"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/cues"
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/widget"
)

// The kinds of the widgets, as nsgo.yaml lists them.
const (
	// Study is the streak and what is due: small on the Home Screen, a ring on the Lock Screen.
	Study = "study"
	// Continue is the film to go on with: small, or medium with the streak and what is due under it.
	Continue = "continue"
)

// ahead is how many midnights the study widget's timeline holds an entry from, past today's.
const ahead = 3

// Day is what the study widget shows from At on: how many days in a row films played, and how many words and lines are due.
type Day struct {
	At          time.Time
	Streak, Due int
}

// Film is the film to go on with: its key on the phone, its computer and ID there, its title, where it was left and how long it is, 0 when not known, and the file of its poster on the phone, "" for a film the phone does not keep.
type Film struct {
	Key, Desktop, ID, Title string
	At, Length              time.Duration
	PosterFile              string
}

// Percent is how far the film played, from 0 to 100.
func (f Film) Percent() int {
	if f.Length <= 0 {
		return 0
	}
	return min(int(f.At*100/f.Length), 100)
}

// Snapshot is what the widgets show of the session, Days today's first and then each midnight's, and Film the film to go on with, a zero Key for none.
type Snapshot struct {
	Days [ahead + 1]Day
	Film Film
}

// Read is what the widgets show of s at now. Read it in a binding, which follows the films, the learning state and the time watched.
func Read(s *session.Session, now time.Time) Snapshot {
	var snap Snapshot
	watched, items := s.Watched(), s.LearnItems()
	for i := range snap.Days {
		at := now
		if i > 0 {
			at = time.Date(now.Year(), now.Month(), now.Day()+i, 0, 0, 0, 0, now.Location())
		}
		snap.Days[i] = Day{At: at, Streak: watched.Streak(at), Due: len(learning.Due(items, at))}
	}
	if list := catalog.Continue(s.Entries(), 1); len(list) > 0 {
		e := list[0]
		snap.Film = Film{Key: e.Key(), Desktop: e.Desktop, ID: e.Film.ID, Title: e.Film.Title,
			At: e.Position.At, Length: time.Duration(e.Film.DurationMS) * time.Millisecond}
		if f, ok := s.Film(e.Key()); ok && f.Poster != "" {
			snap.Film.PosterFile = filepath.Join(s.Store().FilmDir(f.Desktop, f.Film.ID), f.Poster)
		}
	}
	return snap
}

// Shown is what the widgets show: the snapshot, with the line the film was left on, its translation where it has one, and the poster of a film the phone does not keep, as fetched this session.
type Shown struct {
	Snapshot
	Line   string
	Poster []byte
}

// Load is snap with what it takes the disk to read: the film's line, from the lines the phone keeps of it, and its poster. Call it off the engine's goroutine.
func Load(s *session.Session, snap Snapshot) Shown {
	w := Shown{Snapshot: snap}
	f := snap.Film
	if f.Key == "" {
		return w
	}
	if f.PosterFile == "" {
		w.Poster = s.KnownPoster(f.Key)
	}
	list, ok, _ := s.Store().Cues(f.Desktop, f.ID)
	if !ok {
		return w
	}
	lines := cues.FromSync(list)
	i := cues.Index(lines, f.At)
	if i < 0 {
		// Left between two lines: the one heard last.
		i = cues.Prev(lines, f.At)
	}
	if i >= 0 {
		w.Line = lines[i].Translation
		if w.Line == "" {
			w.Line = lines[i].Original
		}
	}
	return w
}

// Keep writes the widgets' timelines with publish (widget.Update) as what they show changes, in the language the app shows, the first time once the session has read the store. The disk is read and the timelines written on a goroutine of its own, which takes the latest change and drops the ones it has not reached.
// TODO(nsgo#262): a tap on a widget only opens the app: NSGo's iOS scene takes no scene:openURLContexts:, so a widget's Entry.URL would never reach the app; give the study widget a link to the review and the Continue widget one to the player once the app's own links reach it on iOS https://github.com/nsgo-dev/nsgo/issues/262
func Keep(cx *core.Scope, s *session.Session, publish func(kind string, entries ...widget.Entry) error) {
	changes := make(chan change, 1)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case c := <-changes:
				w := Load(s, c.snap)
				for kind, entries := range Entries(c.m, w) {
					if err := publish(kind, entries...); err != nil {
						log.Printf("widgets: %s: %v", kind, err)
					}
				}
			}
		}
	}()
	cx.Cleanup(func() { close(done) })
	var last change
	// The days are dated from the time of the read, and the study widget's timeline runs out a few midnights on: they are read again each time the app shows after it was in the background, so days that passed meanwhile do not leave the widgets on the last of them.
	shown := core.NewMemo(cx, func() bool { return cx.Env().Lifecycle() != core.LifecycleBackground })
	cx.Effect(func() {
		_ = shown.Get()
		if !s.Loaded() {
			return
		}
		c := change{m: locales.For(cx), snap: Read(s, s.Now())}
		// The days are dated from now, which changes on every read: what the widgets show changes only with what the days hold.
		if same(c, last) {
			return
		}
		last = c
		select {
		case <-changes:
		default:
		}
		changes <- c
	})
}

// change is what the widgets show, in the language of m.
type change struct {
	m    locales.Messages
	snap Snapshot
}

// same reports whether two changes show the same, whenever they were read: the same messages, film and streaks and counts on the same days.
func same(a, b change) bool {
	if a.m != b.m || a.snap.Film != b.snap.Film {
		return false
	}
	for i := range a.snap.Days {
		x, y := a.snap.Days[i], b.snap.Days[i]
		if x.Streak != y.Streak || x.Due != y.Due || (i > 0 && !x.At.Equal(y.At)) {
			return false
		}
	}
	return true
}

// The colors of the widgets: the flame of the streak, and the white of the texts over a poster.
var (
	flame = widget.RGB(0xFF9500)
	white = widget.RGB(0xFFFFFF)
)

// Entries are the timelines of the widgets of each kind, in the language of m: the study widget's an entry a day, the Continue widget's one.
func Entries(m locales.Messages, w Shown) map[string][]widget.Entry {
	study := make([]widget.Entry, len(w.Days))
	for i, d := range w.Days {
		study[i] = widget.Entry{View: streakView(m, d), Families: map[widget.Family]widget.View{widget.Circular: ringView(m, d)}}
		if i > 0 {
			study[i].Date = d.At
		}
	}
	today := w.Days[0]
	cont := widget.Entry{View: noFilmView(m)}
	if w.Film.Key != "" {
		cont = widget.Entry{View: filmView(m, w), Families: map[widget.Family]widget.View{widget.Medium: wideFilmView(m, w, today)}}
		if bg := poster(w, true); bg != nil {
			cont.Background = bg
		}
	}
	return map[string][]widget.Entry{Study: study, Continue: {cont}}
}

// streakView is the small study widget: the streak under a flame, and what is due at its foot.
func streakView(m locales.Messages, d Day) widget.View {
	return widget.Column{Style: widget.Style{AlignItems: widget.AlignStart, FillWidth: true, FillHeight: true, Gap: 2}, Children: []widget.View{
		widget.Row{Style: widget.Style{Gap: 4}, Children: []widget.View{
			widget.Symbol{Name: "flame.fill", Style: widget.Style{Color: flame, FontSize: 13, Accented: true}},
			widget.Text{Content: m.WidgetStreak(), Style: widget.Style{TextStyle: widget.Caption, FontWeight: core.FontWeightBold, Color: widget.Secondary, MaxLines: 1}},
		}},
		widget.Row{Style: widget.Style{Gap: 3, AlignItems: widget.AlignEnd}, Children: []widget.View{
			widget.Text{Content: strconv.Itoa(d.Streak), Style: widget.Style{FontSize: 40, FontWeight: core.FontWeight(800), FontVariant: core.FontTabularNums, MaxLines: 1, MinScale: 0.5}},
			widget.Text{Content: m.WidgetDays(d.Streak), Style: widget.Style{FontSize: 15, FontWeight: core.FontWeightBold, MaxLines: 1, Padding: core.Edges{Bottom: 6}}},
		}},
		widget.Spacer{},
		widget.Text{Content: m.WidgetDue(d.Due), Style: widget.Style{TextStyle: widget.Footnote, FontWeight: core.FontWeightSemibold, MaxLines: 1, MinScale: 0.8}},
		widget.Text{Content: m.WidgetTapToReview(), Style: widget.Style{TextStyle: widget.Caption2, Color: widget.Secondary, MaxLines: 1}},
	}}
}

// ringView is the Lock Screen's round widget: how many words and lines are due.
func ringView(m locales.Messages, d Day) widget.View {
	return widget.Column{Style: widget.Style{Gap: 0}, Children: []widget.View{
		widget.Text{Content: strconv.Itoa(d.Due), Style: widget.Style{FontSize: 22, FontWeight: core.FontWeight(800), FontVariant: core.FontTabularNums, MaxLines: 1, MinScale: 0.5, Accented: true}},
		widget.Text{Content: m.WidgetDueShort(), Style: widget.Style{FontSize: 10, FontWeight: core.FontWeightBold, MaxLines: 1, MinScale: 0.6}},
	}}
}

// noFilmView is the Continue widget while no film was left partway.
func noFilmView(m locales.Messages) widget.View {
	return widget.Column{Style: widget.Style{AlignItems: widget.AlignStart, FillWidth: true, FillHeight: true, Gap: 4}, Children: []widget.View{
		widget.Symbol{Name: "play.rectangle.on.rectangle", Style: widget.Style{Color: widget.Accent, FontSize: 22, Accented: true}},
		widget.Spacer{},
		widget.Text{Content: m.WidgetNoFilm(), Style: widget.Style{TextStyle: widget.Subheadline, FontWeight: core.FontWeightBold, MaxLines: 2}},
		widget.Text{Content: m.WidgetNoFilmHint(), Style: widget.Style{TextStyle: widget.Caption2, Color: widget.Secondary, MaxLines: 2}},
	}}
}

// filmView is the small Continue widget: over the film's poster, on a scrim at its foot, the line it was left on, else its title, and how far it played.
func filmView(m locales.Messages, w Shown) widget.View {
	text := w.Line
	if text == "" {
		text = w.Film.Title
	}
	return widget.Column{Style: widget.Style{FillWidth: true, FillHeight: true, AlignItems: widget.AlignStart, Justify: widget.AlignEnd}, Children: []widget.View{
		widget.Column{Style: widget.Style{FillWidth: true, AlignItems: widget.AlignStart, Gap: 4, Padding: core.All(8), CornerRadius: 10, Background: widget.RGBA(0x000000, 0.55), Color: white}, Children: []widget.View{
			widget.Text{Content: m.WidgetContinue(), Style: widget.Style{TextStyle: widget.Caption2, FontWeight: core.FontWeightBold}},
			widget.Text{Content: text, Style: widget.Style{FontSize: 13, FontWeight: core.FontWeightBold, MaxLines: 2}},
			widget.Progress{Value: float64(w.Film.Percent()) / 100, Style: widget.Style{Color: white, Accented: true}},
		}},
	}}
}

// wideFilmView is the medium Continue widget: the poster beside how far the film played, its title and the line it was left on, and the streak and what is due at its foot.
func wideFilmView(m locales.Messages, w Shown, today Day) widget.View {
	texts := []widget.View{
		widget.Text{Content: m.WidgetContinuePercent(w.Film.Percent()), Style: widget.Style{TextStyle: widget.Caption2, FontWeight: core.FontWeightBold, Color: widget.Accent, Accented: true, MaxLines: 1}},
		widget.Text{Content: w.Film.Title, Style: widget.Style{TextStyle: widget.Subheadline, FontWeight: core.FontWeight(800), MaxLines: 2}},
	}
	if w.Line != "" {
		texts = append(texts, widget.Text{Content: w.Line, Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary, MaxLines: 2}})
	}
	texts = append(texts, widget.Spacer{},
		widget.Text{Content: m.WidgetFoot(today.Due, today.Streak), Style: widget.Style{TextStyle: widget.Caption2, Color: widget.Secondary, MaxLines: 1, MinScale: 0.8}})
	row := []widget.View{}
	if p := poster(w, false); p != nil {
		row = append(row, p)
	}
	row = append(row, widget.Column{Style: widget.Style{FillWidth: true, FillHeight: true, AlignItems: widget.AlignStart, Gap: 3}, Children: texts})
	return widget.Row{Style: widget.Style{FillWidth: true, FillHeight: true, Gap: 12}, Children: row}
}

// poster is the film's poster: as the small widget's background, covering it, or as the medium widget's picture beside its texts; nil for none.
func poster(w Shown, background bool) widget.View {
	if w.Film.PosterFile == "" && len(w.Poster) == 0 {
		return nil
	}
	img := widget.Image{File: w.Film.PosterFile, Data: w.Poster, Cover: true}
	if w.Film.PosterFile != "" {
		img.Data = nil
	}
	if !background {
		img.Style = widget.Style{Width: 110, FillHeight: true, CornerRadius: 12}
	}
	return img
}
