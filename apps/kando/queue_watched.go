package main

import (
	"time"

	psync "kando/packages/sync"
)

// The film watched is the one the queue makes first, and nearest where it is watched (queuePreferOpen, queuePlaying): the film open in this computer's player, or a film being made that a paired phone plays (state_phonesync_watch.go). Each player claims its film as its user opens it, plays it or moves its playhead, and the film watched is the one claimed last, so the queue follows the person to whichever screen they turned to. A phone keeps its claim, and moves where it watches, every psync.WatchEvery while its film is open, without taking the film back from a player that claimed since; psync.WatchLease after its last word it no longer counts. Neither this computer's reports, about four times a second while a film plays, nor a phone's regular word take the film from the other, so two players watching two films do not have the queue switch between them.

// queueViewer is a paired phone watching a film being made: the film, where it is watched, when its user last claimed it, and until when it counts.
type queueViewer struct {
	film    string
	head    time.Duration
	claimed time.Time
	until   time.Time
}

// queueDesk is this computer's player as the queue last saw it, to tell its user's claims: the film open, whether it was held, how many seeks it had, and when it was last claimed.
type queueDesk struct {
	key     string
	paused  bool
	seeks   int
	claimed time.Time
}

// queueWatching is the film watched (queueWatched): its key, where it is watched when that is known (placed), and the phone watching it, "" for this computer's player.
type queueWatching struct {
	film   string
	head   time.Duration
	placed bool
	phone  string
}

// queueNow is the queue's clock: a test's, or the time.
func (a *app) queueNow() time.Time {
	if a.queue.clock != nil {
		return a.queue.clock()
	}
	return time.Now()
}

// queueWatched is the film watched: the one claimed last, by this computer's player while a film is open in it, or by a phone whose claim still counts; this computer's player when they claimed at the same time. ok is false while nothing is watched.
func (a *app) queueWatched() (queueWatching, bool) {
	now := a.queueNow()
	var out queueWatching
	var claimed time.Time
	found := false
	if a.openKey != "" {
		out, claimed, found = queueWatching{film: a.openKey}, a.queue.desk.claimed, true
		if p := a.player.play; p != nil {
			out.head, out.placed = p.head, true
		}
	}
	for phone, v := range a.queue.viewers {
		if now.After(v.until) {
			continue
		}
		if !found || v.claimed.After(claimed) || v.claimed.Equal(claimed) && out.phone != "" && phone < out.phone {
			out, claimed, found = queueWatching{film: v.film, head: v.head, placed: true, phone: phone}, v.claimed, true
		}
	}
	return out, found
}

// queueDeskSeen notes this computer's player as it is now, and a claim of its user since it was last seen: a film opened, played after it was held, or moved by a seek of the user's (playback.keepSeek), not a looped line going back to its start.
func (a *app) queueDeskSeen() {
	d := &a.queue.desk
	paused, seeks, users := true, 0, false
	if p := a.player.play; p != nil {
		paused, seeks, users = p.paused, p.seeks, p.keepSeek
	}
	if a.openKey != "" && (a.openKey != d.key || d.paused && !paused || seeks > d.seeks && users) {
		d.claimed = a.queueNow()
	}
	d.key, d.paused, d.seeks = a.openKey, paused, seeks
}

// queueWatchedBy notes that phone watches film, being made, at at, claimed by its user when claim is set or when it watched another film before; while that makes it the film watched and its job runs, the job makes the lines there first, as from this computer's player (a claim moves the playhead as the player's reports do, its regular word only keeps it where the phone is). The queue looks at the films again once the claim no longer counts (queueLease). It reports whether this phone now watches the film watched.
func (a *app) queueWatchedBy(phone, film string, at time.Duration, claim bool) bool {
	now := a.queueNow()
	if a.queue.viewers == nil {
		a.queue.viewers = map[string]queueViewer{}
	}
	v, had := a.queue.viewers[phone]
	if !had || v.film != film || now.After(v.until) {
		claim = true
	}
	v.film, v.head, v.until = film, at, now.Add(psync.WatchLease)
	if claim {
		v.claimed = now
	}
	a.queue.viewers[phone] = v
	a.queueLease()
	w, ok := a.queueWatched()
	if !ok || w.phone != phone {
		return false
	}
	if watch := a.queue.watch; watch != nil && film == a.queue.active {
		if claim {
			watch.move(at)
		} else {
			watch.keep(at)
		}
	}
	return true
}

// queueLease has the queue look at the films again once the first phone's claim stops counting, dropping those that did, so a film no phone watches any longer gives way to the one open on this computer.
func (a *app) queueLease() {
	now := a.queueNow()
	var next time.Time
	for phone, v := range a.queue.viewers {
		switch {
		case now.After(v.until):
			delete(a.queue.viewers, phone)
		case next.IsZero() || v.until.Before(next):
			next = v.until
		}
	}
	if a.queue.lease != nil {
		a.queue.lease.Stop()
		a.queue.lease = nil
	}
	if next.IsZero() {
		return
	}
	update := a.update
	a.queue.lease = time.AfterFunc(next.Sub(now)+time.Millisecond, func() {
		update(func() {
			a.queueLease()
			a.queuePreferOpen()
		})
	})
}
