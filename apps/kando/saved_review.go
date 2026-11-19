package main

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/saved"
	"kando/apps/kando/study"

	"github.com/nsgo-dev/nsgo/core"
)

// The review of the saved lines and the words of the vocabulary due (study.Review): what it goes through, the sound of each, which is the one kept in the library when it was saved (saved.StoredSound) or else cut with ffmpeg off the engine's goroutine and kept there from then on, or, where the library cannot keep it, cut into a folder of the review's own that goes when the review ends; and the answers, which schedule each in the library's study record (saved.Book.Grade, GradeWord).

// reviewItem is a saved line or a word of the vocabulary under review, by its id: savedID for a line, the word's own for a word.
type reviewItem struct {
	word bool
	id   string
}

// savedReview is the review open now: what is left, the one shown first; whether its translation shows; how many were answered; and the sound of each, by its id, its file once cut or "" for one that has none, which playing and plays start. dir is the review's own folder for sounds the library cannot keep, and ctx the cutting's, which cancel stops as the review ends.
type savedReview struct {
	open     bool
	queue    []reviewItem
	revealed bool
	reviewed int
	clips    map[string]string
	cutting  map[string]bool
	playing  bool
	plays    int
	dir      string
	ctx      context.Context
	cancel   context.CancelFunc
	release  func()
}

// savedReviewOverlay is the review over the window while it is open (overlays). It is built as it opens.
func (a *app) savedReviewOverlay(*core.Scope) core.Node {
	return core.Show{
		When: live.Func(a, func() bool { return a.saved.review.open }),
		Then: func(*core.Scope) core.Node { return study.Review{App: a} },
	}
}

// DueCount is how many saved lines and words of the vocabulary are due for review now.
func (a *app) DueCount() int {
	book, now := a.savedBook(), a.saved.now()
	return len(book.Due(now)) + len(book.DueWords(now))
}

// savedDue is what the review goes through: the saved lines and the words due now, the one due longest first, a line before a word due at the same time.
func (a *app) savedDue() []reviewItem {
	book, now := a.saved.book, a.saved.now()
	type due struct {
		at   time.Time
		item reviewItem
	}
	var all []due
	for _, i := range book.Due(now) {
		all = append(all, due{book.Lines[i].Due, reviewItem{id: savedID(book.Lines[i])}})
	}
	for _, i := range book.DueWords(now) {
		all = append(all, due{book.Words[i].Due, reviewItem{word: true, id: book.Words[i].ID}})
	}
	slices.SortStableFunc(all, func(x, y due) int { return x.at.Compare(y.at) })
	out := make([]reviewItem, len(all))
	for k, d := range all {
		out[k] = d.item
	}
	return out
}

// StartReview is N, the Study menu's Review and the Saved Lines sheet's and the notes tab's Review: the lines and words due, one at a time, over the window, with the film paused under it.
func (a *app) StartReview() {
	if a.saved.review.open {
		return
	}
	a.savedSync()
	a.CloseSaved()
	if p := a.player.play; p != nil && p.Playing() {
		p.toggle()
	}
	r := savedReview{open: true, clips: map[string]string{}, cutting: map[string]bool{}, release: a.holdModal(), queue: a.savedDue()}
	if dir, err := os.MkdirTemp("", "kando-review-*"); err == nil {
		r.dir = dir
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	a.saved.review = r
	a.savedReviewCut()
}

// savedReviewLine is the line of item, the saved line itself or the line a word was looked up in (saved.WordLine), ok false once it is not in the book.
func (a *app) savedReviewLine(item reviewItem) (saved.Line, bool) {
	book := a.saved.book
	if item.word {
		if i := book.WordByID(item.id); i >= 0 {
			return saved.WordLine(book.Words[i]), true
		}
		return saved.Line{}, false
	}
	if i := a.savedFind(item.id); i >= 0 {
		return book.Lines[i], true
	}
	return saved.Line{}, false
}

// savedReviewCut finds the sound of what is shown and of the one after it, unless they have theirs or are being cut: the sound kept in the library, or one being kept as it was just saved, which comes once it is (savedSoundKept), or else it is cut now. A sound cut here is kept in the library too, or in the review's own folder where the library cannot keep it.
func (a *app) savedReviewCut() {
	r := &a.saved.review
	ctx, ffmpeg, update := r.ctx, a.ffmpegPath(), a.update
	for k, item := range r.queue {
		if k > 1 {
			break
		}
		id := item.id
		if _, ok := r.clips[id]; ok || r.cutting[id] {
			continue
		}
		l, ok := a.savedReviewLine(item)
		if !ok {
			r.clips[id] = ""
			continue
		}
		kept := saved.StoredSound(a.saved.book, l.ID)
		if saved.HasSound(kept) {
			a.savedReviewSound(id, kept)
			continue
		}
		if a.saved.keeping[l.ID] {
			continue
		}
		video := a.saved.book.Abs(l)
		if _, err := os.Stat(video); err != nil || ffmpeg == "" || l.Video == "" || update == nil {
			r.clips[id] = ""
			continue
		}
		delay := a.player.delays[video]
		own := ""
		if r.dir != "" {
			own = filepath.Join(r.dir, saved.SoundName(l))
		}
		r.cutting[id] = true
		go func() {
			out := cmp.Or(kept, own)
			err := errors.New("nowhere to keep the sound")
			if out != "" {
				err = saved.KeepSound(ctx, ffmpeg, video, l, delay, out)
			}
			if err != nil && ctx.Err() == nil && out == kept && own != "" {
				out = own
				err = saved.KeepSound(ctx, ffmpeg, video, l, delay, out)
			}
			update(func() {
				if ctx.Err() != nil || !a.saved.review.open {
					return
				}
				r := &a.saved.review
				delete(r.cutting, id)
				if err != nil {
					out = ""
				}
				a.savedReviewSound(id, out)
			})
		}()
	}
}

// savedReviewSound gives what the review shows by id its sound, out, "" for none, and plays it if it is shown now.
func (a *app) savedReviewSound(id, out string) {
	r := &a.saved.review
	r.clips[id] = out
	if out != "" && len(r.queue) > 0 && r.queue[0].id == id {
		r.playing = true
		r.plays++
	}
}

// ReviewCard is what the review shows now: a saved line, its note shown with its translation, or a word with its reading and the line it was looked up in, its meaning and the line's translation shown once revealed.
func (a *app) ReviewCard() study.Card {
	r := a.saved.review
	c := study.Card{Reviewed: r.reviewed, Left: len(r.queue), Done: len(r.queue) == 0}
	if c.Done {
		return c
	}
	item := r.queue[0]
	clip, cut := r.clips[item.id]
	c.Clip = clip
	c.NoAudio = cut && clip == ""
	c.Playing = r.playing && clip != ""
	c.Plays = r.plays
	if item.word {
		book := a.saved.book
		i := book.WordByID(item.id)
		if i < 0 {
			return c
		}
		w := book.Words[i]
		c.Word = true
		c.Film, c.Original, c.Reading, c.Translation, c.Start = w.Film, w.Text, w.Reading, w.Meaning, w.Start
		c.Context, c.ContextTranslation = w.Context, w.ContextTranslation
		if f, ok := a.savedFilm(saved.WordLine(w)); ok && f.Name != "" {
			c.Film = f.Name
		}
		c.Revealed = r.revealed || (w.Meaning == "" && w.ContextTranslation == "")
		return c
	}
	i := a.savedFind(item.id)
	if i < 0 {
		return c
	}
	l := a.saved.book.Lines[i]
	c.Film, c.Original, c.Translation, c.Note, c.Start = l.Film, l.Original, l.Translation, l.Note, l.Start
	if f, ok := a.savedFilm(l); ok && f.Name != "" {
		c.Film = f.Name
	}
	// A line with nothing to reveal is answered at once.
	c.Revealed = r.revealed || (l.Translation == "" && l.Note == "")
	return c
}

// ReviewReveal shows the translation of what is under review.
func (a *app) ReviewReveal() { a.saved.review.revealed = true }

// ReviewGrade schedules what is under review by how the learner knew it (saved.Book.Grade, GradeWord) and goes on to the next; one not known comes back at the end of this review.
func (a *app) ReviewGrade(good bool) {
	r := &a.saved.review
	if len(r.queue) == 0 {
		return
	}
	item := r.queue[0]
	r.queue = r.queue[1:]
	book, now := &a.saved.book, a.saved.now()
	graded := false
	if item.word {
		if i := book.WordByID(item.id); i >= 0 {
			w := book.Words[i]
			graded = book.GradeWord(w.Text, w.Spoken, good, now)
		}
	} else if i := a.savedFind(item.id); i >= 0 {
		l := book.Lines[i]
		graded = book.Grade(l.Video, l.Start, good, now)
	}
	if graded {
		a.savedWrite()
		if !good {
			r.queue = append(r.queue, item)
		}
	}
	r.reviewed++
	r.revealed = false
	r.playing = false
	if len(r.queue) > 0 {
		if clip := r.clips[r.queue[0].id]; clip != "" {
			r.playing = true
			r.plays++
		}
	}
	a.savedReviewCut()
}

// ReviewReplay plays the sound of the line under review again from its start.
func (a *app) ReviewReplay() {
	r := &a.saved.review
	if len(r.queue) == 0 || r.clips[r.queue[0].id] == "" {
		return
	}
	r.playing = true
	r.plays++
}

// ReviewClipEnded is told when the line's sound has played to its end.
func (a *app) ReviewClipEnded() { a.saved.review.playing = false }

// CloseReview ends the review: the cutting stops, the sounds' folder goes, and the window's modal flag is given back.
func (a *app) CloseReview() {
	r := a.saved.review
	if r.cancel != nil {
		r.cancel()
	}
	if r.dir != "" {
		_ = os.RemoveAll(r.dir)
	}
	if r.release != nil {
		r.release()
	}
	a.saved.review = savedReview{}
}
