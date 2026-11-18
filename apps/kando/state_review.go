package main

import (
	"errors"
	"time"

	"kando/apps/kando/film"
	"kando/packages/core"
	"kando/packages/media"
)

// The final proofread (终校, core.Proof): a job reads the film's translation through again once it is made, when Settings › 翻译 has it on (media.WithReview), the changes of the kinds the setting lets apply by themselves written into the subtitles and the others waiting. The changes are kept beside the video (media.ProofsPath); the subtitle list's 终校 chip lists the open film's, where each is accepted, turned down or rolled back, and the whole pass accepted or rolled back (film.Reviewer).

// reviewUI is the open film's final proofread: path is the subtitles it is kept beside, and mod when they were last read, read whether they have been, so a job rewriting them, or a decision, has the changes read again; proofs are what the file holds once read (ok false for a file that could not be read, which is then not written over), gen telling a read from one of a film opened before. view is whether the subtitle list shows the changes in place of the lines.
type reviewUI struct {
	path   string
	mod    time.Time
	read   bool
	proofs []core.Proof
	ok     bool
	gen    int
	view   bool
}

func newReviewUI() reviewUI { return reviewUI{} }

// reviewHooks follows the open film's subtitles, whose changes it reads again whenever they are read again.
func (a *app) reviewHooks() areaHooks {
	return areaHooks{afterChange: a.reviewFollow}
}

// reviewFollow reads the changes of the open film's final proofread off the engine's goroutine once its subtitles are read, and again once they are read again after they were rewritten, keeping the ones it holds until then, and reports whether it dropped them for another film's.
func (a *app) reviewFollow() bool {
	subs := a.player.subs
	path := ""
	if a.openKey != "" && subs.key == a.openKey {
		path = subs.path
	}
	r := &a.review
	dropped := false
	if path != r.path {
		dropped = len(r.proofs) > 0 || r.view
		*r = reviewUI{path: path, gen: r.gen + 1}
	}
	if path == "" || !subs.ready || subs.err != "" || r.read && subs.mod.Equal(r.mod) {
		return dropped
	}
	r.mod, r.read = subs.mod, true
	r.gen++
	gen, update := r.gen, a.update
	go func() {
		proofs, err := media.LoadProofs(media.ProofsPath(path))
		update(func() {
			if a.review.gen != gen {
				return
			}
			a.review.proofs, a.review.ok = proofs, err == nil
		})
	}()
	return dropped
}

// Proofs is the open film's changes, each marked Mine when the user corrected its line's translation, which the change then no longer touches.
func (a *app) Proofs() []film.Proof {
	if len(a.review.proofs) == 0 {
		return nil
	}
	subs := a.player.subs
	mine := map[core.Span]bool{}
	for _, l := range subs.from {
		if l.Edit >= 0 && l.Edit < len(subs.edits) && subs.edits[l.Edit].Translation != nil {
			mine[l.Machine] = true
		}
	}
	out := make([]film.Proof, len(a.review.proofs))
	for i, p := range a.review.proofs {
		out[i] = film.Proof{Proof: p, Mine: mine[p.Span]}
	}
	return out
}

// ReviewOn reports whether Settings › 翻译 has the final proofread on.
func (a *app) ReviewOn() bool { return a.prefs.review }

// ProofView is whether the subtitle list shows the changes in place of the lines.
func (a *app) ProofView() *bool { return &a.review.view }

// ProofsBusy reports whether the open film's changes cannot be decided now: they are not read, or could not be, or a job of the film is making or translating it again, which rewrites its subtitles.
func (a *app) ProofsBusy() bool {
	if !a.review.ok || a.review.path == "" || !a.correctLoaded() {
		return true
	}
	item, ok := a.openFilm()
	return ok && item.JobID != "" && item.Stage != "" && !item.Stage.Done()
}

// DecideProof accepts the change at i, or turns it down, or rolls it back when it is live.
func (a *app) DecideProof(i int, accept bool) {
	a.reviewDecide(core.DecideProof(a.review.proofs, i, accept))
}

// AcceptProofs accepts every change that waits; RollBackProofs takes the whole pass back, keeping what the user accepted.
func (a *app) AcceptProofs() { a.reviewDecide(core.AcceptProofs(a.review.proofs)) }

func (a *app) RollBackProofs() {
	a.reviewDecide(core.RollBackProofs(a.review.proofs))
	a.flash(a.messages().ReviewRolledBack())
}

// reviewDecide keeps after as the open film's changes and rewrites its subtitles to match (media.DecideProofs), which are then read again, and says on the picture when they could not be written.
func (a *app) reviewDecide(after []core.Proof) {
	if a.ProofsBusy() {
		return
	}
	if err := media.DecideProofs(a.review.path, a.review.proofs, after); err != nil {
		if errors.Is(err, media.ErrProofsNewer) {
			a.flash(a.messages().ReviewNewer())
		} else {
			a.flash(a.messages().ReviewSaveFailed())
		}
		return
	}
	a.review.proofs = after
	a.reloadOpenSubtitles()
	delete(a.lib.quotes, a.review.path)
	a.libraryHeroQuote()
}
