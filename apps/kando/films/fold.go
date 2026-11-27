package films

import (
	"bytes"
	"slices"
	"time"

	"kando/apps/kando/locales"
	"kando/packages/core"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// The library page folds the films of a work it shows into one card (Works), after the sidebar's filters and the search have chosen the films it shows, so a card holds the work's films that pass them: in a category, the work's films in it; under 未看, those not watched. A work with one film shown is that film's card. A work the user split apart (App.WorkApart) shows a card a film, and plays on as before.

// card is one of the cards of the done films the page shows: a film's, or a work's, Work, with Films its films the page shows, two or more, in the work's order. Key tells it from the others, a film's key or the work's. Film is the film's, or the one the work goes on with (Continue), whose poster is the work's.
type card struct {
	Key   string
	Film  Film
	Work  *Work
	Films []Film
}

func (c card) equal(o card) bool {
	if c.Key != o.Key || c.Film != o.Film || (c.Work == nil) != (o.Work == nil) || !slices.Equal(c.Films, o.Films) {
		return false
	}
	return c.Work == nil || c.Work.Name == o.Work.Name && slices.Equal(c.Work.Films, o.Work.Films)
}

// workCardKey is the key of a work's card, which no film's key can be.
func workCardKey(w *Work) string { return "work\x00" + w.Key }

// fold is the cards of done, the films the page shows in their order, their works' among works folded into one each where it stands first in done: by title (SortTitle), a work stands by its name, in o's direction. all is the library's films, which a work goes on with one of (Continue); apart reports a work the user split apart, whose films keep their cards.
func fold(done, all []Film, works WorkIndex, apart func(*Work) bool, p Played, o Order, tag string) []card {
	shown := map[*Work][]Film{}
	for _, f := range done {
		if w, ok := works.Of(f.Key); ok {
			shown[w] = append(shown[w], f)
		}
	}
	byKey := make(map[string]Film, len(all))
	for _, f := range all {
		byKey[f.Key] = f
	}
	out := make([]card, 0, len(done))
	placed := map[*Work]bool{}
	for _, f := range done {
		w, ok := works.Of(f.Key)
		if !ok || len(shown[w]) < 2 || apart(w) {
			out = append(out, card{Key: f.Key, Film: f})
			continue
		}
		if placed[w] {
			continue
		}
		placed[w] = true
		films := shown[w]
		slices.SortStableFunc(films, func(a, b Film) int { return slices.Index(w.Films, a.Key) - slices.Index(w.Films, b.Key) })
		next, _ := continueAmong(p, workFilms(w, byKey))
		out = append(out, card{Key: workCardKey(w), Film: next, Work: w, Films: films})
	}
	if o.Valid().By == SortTitle && len(out) > 1 {
		col := collate.New(language.Make(tag), collate.Numeric)
		var buf collate.Buffer
		keys := make(map[string][]byte, len(out))
		for _, c := range out {
			keys[c.Key] = col.KeyFromString(&buf, c.name())
		}
		ascending := o.Valid().Ascending()
		slices.SortStableFunc(out, func(a, b card) int {
			c := bytes.Compare(keys[a.Key], keys[b.Key])
			if !ascending {
				c = -c
			}
			return c
		})
	}
	return out
}

// name is what the card is called: the work's name, or the film's.
func (c card) name() string {
	if c.Work != nil {
		return c.Work.Name
	}
	return filmName(c.Film)
}

// cardFilms is the films of cards, a work's in its place, in the cards' order: the order ⌘A and a ⇧-click select in.
func cardFilms(cards []card) []Film {
	var out []Film
	for _, c := range cards {
		if c.Work != nil {
			out = append(out, c.Films...)
		} else {
			out = append(out, c.Film)
		}
	}
	return out
}

// Played is what a work's card reads of how its films were watched: when each last played, and how far, against how long it is (Progress).
type Played interface {
	Progress
	PlayedAt(path string) time.Time
}

// The ways a work goes on (Continue): from the film watched last, stopped partway; with the film after the one watched to its end last, or the first not watched; from its first film, none being watched yet; or from its first again, every one being watched.
const (
	GoOnResume = iota
	GoOnNext
	GoOnStart
	GoOnAgain
)

// Continue is the film of w, among all, a work goes on with, and how (GoOnResume and the others): the film played last when it was stopped partway; else the one after the film played last, a film that failed passed over; else the first not watched, as when films were marked watched (MarkWatched) and none played; else its first, from the start when none was watched and again when every one was. Every film of the work counts, those the page does not show too, so a card under 未看 goes on where the work does.
func Continue(p Played, w *Work, all []Film) (Film, int) {
	byKey := make(map[string]Film, len(all))
	for _, f := range all {
		byKey[f.Key] = f
	}
	return continueAmong(p, workFilms(w, byKey))
}

// workFilms is the films of w, in its order, as byKey has them by key.
func workFilms(w *Work, byKey map[string]Film) []Film {
	films := make([]Film, 0, len(w.Films))
	for _, key := range w.Films {
		if f, ok := byKey[key]; ok {
			films = append(films, f)
		}
	}
	return films
}

// continueAmong is Continue among films, a work's in its order.
func continueAmong(p Played, films []Film) (Film, int) {
	if len(films) == 0 {
		return Film{}, GoOnStart
	}
	last, at := -1, time.Time{}
	for i, f := range films {
		if played := p.PlayedAt(f.VideoPath); !played.IsZero() && played.After(at) {
			last, at = i, played
		}
	}
	if last >= 0 {
		if !watchedOf(p, films[last]).end {
			return films[last], GoOnResume
		}
		for _, f := range films[last+1:] {
			if f.Stage != core.StageFailed && !watchedOf(p, f).end {
				return f, GoOnNext
			}
		}
	} else if !slices.ContainsFunc(films, func(f Film) bool { return watchedOf(p, f).end }) {
		return films[0], GoOnStart
	}
	for _, f := range films {
		if !watchedOf(p, f).end && f.Stage != core.StageFailed {
			return f, GoOnNext
		}
	}
	return films[0], GoOnAgain
}

// unwatched is how many of films were not watched to their end, which a work's card marks.
func unwatched(p Progress, films []Film) int {
	n := 0
	for _, f := range films {
		if !watchedOf(p, f).end {
			n++
		}
	}
	return n
}

// goOnLabel is what the button that goes on with a work says, in the language of m: 继续 with the film's place in the work, 从第一集看 or 从头再看.
func goOnLabel(m locales.Messages, w *Work, f Film, how int) string {
	switch how {
	case GoOnStart:
		return m.FilmsWorkStart()
	case GoOnAgain:
		return m.FilmsWorkAgain()
	}
	return m.FilmsWorkContinue(slices.Index(w.Films, f.Key) + 1)
}
