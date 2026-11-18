package main

import (
	"context"
	"os"
	"path/filepath"

	"kando/apps/kando/live"
	"kando/apps/kando/saved"
	"kando/apps/kando/study"

	"github.com/nsgo-dev/nsgo/core"
)

// The review of the saved lines due (study.Review): the lines it goes through, the sound of each, cut with ffmpeg off the engine's goroutine into a folder of its own that goes when the review ends, and the answers, which schedule each line in the library's study record (saved.Book.Grade).

// savedReview is the review open now: the lines left, by savedID, the one shown first; whether its translation shows; how many were answered; and the sound of each line, its file once cut or "" for one that has none, which playing and plays start. dir is the folder the sounds are cut into, and ctx the cutting's, which cancel stops as the review ends.
type savedReview struct {
	open     bool
	queue    []string
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

// DueCount is how many saved lines are due for review now.
func (a *app) DueCount() int { return len(a.savedBook().Due(a.saved.now())) }

// StartReview is N, the Study menu's Review Saved Lines and the Saved Lines sheet's Review: the lines due, one at a time, over the window, with the film paused under it.
func (a *app) StartReview() {
	if a.saved.review.open {
		return
	}
	a.savedSync()
	a.CloseSaved()
	if p := a.player.play; p != nil && p.Playing() {
		p.toggle()
	}
	r := savedReview{open: true, clips: map[string]string{}, cutting: map[string]bool{}, release: a.holdModal()}
	for _, i := range a.saved.book.Due(a.saved.now()) {
		r.queue = append(r.queue, savedID(a.saved.book.Lines[i]))
	}
	if dir, err := os.MkdirTemp("", "kando-review-*"); err == nil {
		r.dir = dir
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	a.saved.review = r
	a.savedReviewCut()
}

// savedReviewCut cuts the sound of the line shown and of the one after it, unless they are cut or being cut, and plays the one shown once it is.
func (a *app) savedReviewCut() {
	r := &a.saved.review
	ctx, ffmpeg, update := r.ctx, a.ffmpegPath(), a.update
	for k, id := range r.queue {
		if k > 1 {
			break
		}
		if _, ok := r.clips[id]; ok || r.cutting[id] {
			continue
		}
		i := a.savedFind(id)
		if i < 0 || ffmpeg == "" || r.dir == "" {
			r.clips[id] = ""
			continue
		}
		l := a.saved.book.Lines[i]
		video := a.saved.book.Abs(l)
		if _, err := os.Stat(video); err != nil {
			r.clips[id] = ""
			continue
		}
		from, to := saved.ClipSpan(l, a.player.delays[video])
		out := filepath.Join(r.dir, saved.SoundName(l))
		r.cutting[id] = true
		go func() {
			err := saved.CutClip(ctx, ffmpeg, video, from, to, out)
			update(func() {
				if ctx.Err() != nil || !a.saved.review.open {
					return
				}
				r := &a.saved.review
				delete(r.cutting, id)
				if err != nil {
					out = ""
				}
				r.clips[id] = out
				if out != "" && len(r.queue) > 0 && r.queue[0] == id {
					r.playing = true
					r.plays++
				}
			})
		}()
	}
}

// ReviewCard is the line the review shows now.
func (a *app) ReviewCard() study.Card {
	r := a.saved.review
	c := study.Card{Reviewed: r.reviewed, Left: len(r.queue), Done: len(r.queue) == 0}
	if c.Done {
		return c
	}
	id := r.queue[0]
	i := a.savedFind(id)
	if i < 0 {
		return c
	}
	l := a.saved.book.Lines[i]
	clip, cut := r.clips[id]
	c.Film, c.Original, c.Translation, c.Start = l.Film, l.Original, l.Translation, l.Start
	if f, ok := a.savedFilm(l); ok && f.Name != "" {
		c.Film = f.Name
	}
	// A line with no translation has nothing to reveal: it is answered at once.
	c.Revealed = r.revealed || l.Translation == ""
	c.Clip = clip
	c.NoAudio = cut && clip == ""
	c.Playing = r.playing && clip != ""
	c.Plays = r.plays
	return c
}

// ReviewReveal shows the translation of the line under review.
func (a *app) ReviewReveal() { a.saved.review.revealed = true }

// ReviewGrade schedules the line under review by how the learner knew it (saved.Book.Grade) and goes on to the next; a line not known comes back at the end of this review.
func (a *app) ReviewGrade(good bool) {
	r := &a.saved.review
	if len(r.queue) == 0 {
		return
	}
	id := r.queue[0]
	r.queue = r.queue[1:]
	if i := a.savedFind(id); i >= 0 {
		l := a.saved.book.Lines[i]
		a.saved.book.Grade(l.Video, l.Start, good, a.saved.now())
		a.savedWrite()
		if !good {
			r.queue = append(r.queue, id)
		}
	}
	r.reviewed++
	r.revealed = false
	r.playing = false
	if len(r.queue) > 0 {
		if clip := r.clips[r.queue[0]]; clip != "" {
			r.playing = true
			r.plays++
		}
	}
	a.savedReviewCut()
}

// ReviewReplay plays the sound of the line under review again from its start.
func (a *app) ReviewReplay() {
	r := &a.saved.review
	if len(r.queue) == 0 || r.clips[r.queue[0]] == "" {
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
