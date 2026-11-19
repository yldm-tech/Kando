package main

import "kando/apps/kando/films"

// The videos of several films selected move to a storage location, or back to the library, one after another, from the selection's menu and the File menu (storedKeys): one move at a time, as a film's alone, the message bar saying which of how many with the copy's progress and 取消, which stops the copy going on and leaves the films after it where they are. A film that cannot move, as one being processed, is left where it is and the others go on; a copy that fails stops the films after it too, as the location that failed them is likely to fail them all, and they stay where they were. The bar then says once how it went.

// storedBatch is the move of several films' videos to the location place, or back to the library (back), one after another: items, those still to begin; at, how many were begun of total; moved, how many moved; failed, how many did not and reason, why the first did not, a sentence in the interface's language; stopped, whether 取消 stopped it, and broke, whether a copy failed, which leaves the films after it where they are.
type storedBatch struct {
	items         []films.Film
	place         string
	back          bool
	at, total     int
	moved, failed int
	reason        string
	stopped       bool
	broke         bool
}

// storedOutcome is how the move of a film's video ended (storedEnded).
type storedOutcome int

const (
	// storedDone is a video moved.
	storedDone storedOutcome = iota
	// storedStopped is a move 取消 stopped, the video where it was.
	storedStopped
	// storedRefused is a video that could not move then, as its film was being processed meanwhile, but that the films after it can.
	storedRefused
	// storedFailed is a copy or what follows it that failed, the video where it was.
	storedFailed
)

// storedEnded ends run as out with text, what the message bar says of it in the interface's language: a film's alone says text, as a warning when it did not move; one of several counts it and goes on with the next (storedStep).
func (a *app) storedEnded(run *storedRun, out storedOutcome, text string) {
	b := run.batch
	if b == nil {
		switch out {
		case storedDone, storedStopped:
			a.tell(text)
		default:
			a.warn(text, "")
		}
		return
	}
	switch out {
	case storedDone:
		b.moved++
	case storedStopped:
		b.stopped = true
	case storedFailed:
		b.broke = true
		fallthrough
	default:
		b.fail(text)
	}
	a.storedStep(b)
}

// fail counts a film of b that did not move, keeping why the first did not.
func (b *storedBatch) fail(reason string) {
	b.failed++
	if b.reason == "" {
		b.reason = reason
	}
}

// MoveVideosTo moves the videos of items, a film or several selected, to the location of ID id, one after another, as 移动视频到 ▸ of the selection's menu and the File menu do. A film's alone moves as MoveVideoTo moves it; of several, those whose videos are already on a location are left out.
func (a *app) MoveVideosTo(items []films.Film, id string) {
	var left []films.Film
	for _, item := range items {
		if _, stored := a.storedOf(item.VideoPath); !stored {
			left = append(left, item)
		}
	}
	switch {
	case len(left) == 0:
		return
	case len(left) == 1:
		a.MoveVideoTo(left[0], id)
		return
	case a.stored.run != nil:
		a.warn(a.messages().StoredBusy(), "")
		return
	}
	a.storedStep(&storedBatch{items: left, place: id, total: len(left)})
}

// MoveVideosBack moves the videos of items that are on a storage location back to the library, one after another, as 将视频移回资料库 of the selection's menu and the File menu do; a film's alone moves as MoveVideoBack moves it.
func (a *app) MoveVideosBack(items []films.Film) {
	back := a.storedOn(items)
	switch {
	case len(back) == 0:
		return
	case len(back) == 1:
		a.MoveVideoBack(back[0])
		return
	case a.stored.run != nil:
		a.warn(a.messages().StoredBusy(), "")
		return
	}
	a.storedStep(&storedBatch{items: back, back: true, total: len(back)})
}

// storedOn is those of items whose videos are on a storage location.
func (a *app) storedOn(items []films.Film) []films.Film {
	var out []films.Film
	for _, item := range items {
		if _, stored := a.storedOf(item.VideoPath); stored {
			out = append(out, item)
		}
	}
	return out
}

// storedStep begins the move of b's next film, until one is copying, whose end goes on from here (storedEnded), or none is left, b was stopped or a copy failed, when it ends (storedBatchEnded). A film that cannot begin is counted and the next goes on.
func (a *app) storedStep(b *storedBatch) {
	for len(b.items) > 0 && !b.stopped && !b.broke {
		item := b.items[0]
		b.items = b.items[1:]
		b.at++
		var err error
		if b.back {
			err = a.storedBeginBack(item, b)
		} else {
			err = a.storedBeginTo(item, b.place, b)
		}
		if err == nil && a.stored.run != nil {
			return
		}
		if err == nil {
			// Moved back meanwhile: nothing to do, which counts as done.
			b.moved++
			continue
		}
		b.fail(err.Error())
	}
	a.storedBatchEnded(b)
}

// storedBatchEnded reads the library's folder again once some videos of b moved (storedRefresh), and says in the message bar how b ended, in the interface's language: every video moved; how many of how many and why the first that did not move did not; that it was stopped, and how many had moved; or, when none moved, why the first did not.
func (a *app) storedBatchEnded(b *storedBatch) {
	if b.moved > 0 {
		a.refreshLibrary()
	}
	m := a.messages()
	place := a.storedName(m, b.place)
	switch {
	case b.stopped && b.back:
		a.tell(m.StoredStoppedBackMany(b.moved))
	case b.stopped:
		a.tell(m.StoredStoppedMany(b.moved, place))
	case b.moved == 0 && b.reason != "":
		a.warn(b.reason, "")
	case b.failed > 0 || b.broke:
		if b.back {
			a.warn(m.StoredMovedBackSome(b.moved, b.total, b.reason), "")
		} else {
			a.warn(m.StoredMovedSome(b.moved, b.total, place, b.reason), "")
		}
	case b.back:
		a.tell(m.StoredMovedBackMany(b.moved))
	default:
		a.tell(m.StoredMovedMany(b.moved, place))
	}
}
