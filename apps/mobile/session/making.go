package session

import (
	"cmp"
	"context"
	"maps"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"kando/apps/mobile/loopback"
	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// Films being made (sync.Film.Making): a computer lists the films whose subtitles it is still making to a phone that asks, and the phone plays one on the computer's network as it is made, its lines coming in as they are made, nearest where it is watched first. The phone keeps it as a film kept to stream (store.Film.Streamed, Stream), so nothing of its video is downloaded: the system's player plays it from a URL of the phone's loopback server (kando/apps/mobile/loopback), which forwards each request to the computer (sync.Client.ServeVideo), the player being unable to pin the computer's certificate or send the phone's token. While the player shows it, the phone tells the computer where it is watched (WatchMaking, sync.OpFilmWatch): at once as its user opens, plays, holds or moves it, which has the computer make it first, and every sync.WatchEvery meanwhile, which keeps that and moves where the lines are made; each answer says how far the film has come, so the phone reads its lines again when they changed, plays its video in place of a link's sound once it comes, and once the film is done takes its record as any film's. Once done, the film streams from its computer on its network as any film kept to stream, and from the storage away from it.

// making is the films being made that the player watches.
type making struct {
	// at is how far each film watched has come, by key, as its computer last said: what StreamLink serves of it among them.
	at *core.Signal[map[string]sync.FilmWatching]
	// watchers are the films the player watches, by key. Engine goroutine only.
	watchers map[string]*Watcher
	// server is the loopback server, started the first time a film streams from its computer or from a source (loopback).
	server *loopback.Server
	// fromStorage is the films the player could not open from their computer this session, by key, which stream from the storage instead (StreamFailed).
	fromStorage *core.Signal[map[string]bool]
}

func newMaking(cx *core.Scope) *making {
	m := &making{at: core.NewSignalFunc(cx, map[string]sync.FilmWatching{}, nil), watchers: map[string]*Watcher{}, fromStorage: core.NewSignalFunc(cx, map[string]bool{}, nil)}
	cx.Cleanup(func() {
		if m.server != nil {
			m.server.Close()
		}
	})
	return m
}

// OnNetwork reports whether the phone reaches desktop on its network now, as it follows the computer's queue: it answers there, not through the storage. Read it in a binding.
func (s *Session) OnNetwork(desktop string) bool {
	q := s.Queue(desktop)
	return q.Reach == Online && !q.Relayed && s.Client(desktop) != nil
}

// lanLink is the URL the player streams film f, kept to stream, from on its computer's network, "" away from it or while nothing of a film being made plays yet. A link's sound plays at a URL of its own until the video comes, so the player opens the video anew, where it was (WatchMaking). Read it in a binding.
func (s *Session) lanLink(f store.Film) string {
	if !s.OnNetwork(f.Desktop) {
		return ""
	}
	plays := sync.PlaysVideo
	if f.Film.Making {
		w, ok := s.making.at.Get()[f.Key()]
		if !ok {
			return ""
		}
		plays = w.Plays
	}
	name := "video.mp4"
	switch plays {
	case "":
		return ""
	case sync.PlaysSound:
		name = "sound.m4a"
	}
	srv := s.loopback()
	if srv == nil {
		return ""
	}
	c, id := s.Client(f.Desktop), f.Film.ID
	return srv.URL(f.Key()+"/"+plays, name, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c.ServeVideo(w, r, id) }))
}

// loopback is the loopback server, started the first time a film streams from its computer or a source; nil when it cannot start. Engine goroutine only.
func (s *Session) loopback() *loopback.Server {
	m := s.making
	if m.server == nil {
		srv, err := loopback.Start()
		if err != nil {
			return nil
		}
		m.server = srv
	}
	return m.server
}

// StreamFailed is the player failing to open the film kept under key from where it streams: one it could not open from its computer on its network streams from the storage for the rest of the session, when the storage holds its video, and StreamFailed reports true, as the player opens it again from there (StreamLink); otherwise it reports false, and the film cannot play. Engine goroutine only.
func (s *Session) StreamFailed(key string) bool {
	f, ok := s.Film(key)
	if !ok || !s.OnNetwork(f.Desktop) || s.making.fromStorage.GetUntracked()[key] || s.remote.shelf.GetUntracked()[f.Desktop].films.VideoLink(f.Film.ID) == "" {
		return false
	}
	from := maps.Clone(s.making.fromStorage.GetUntracked())
	from[key] = true
	s.making.fromStorage.Set(from)
	return true
}

// streamMaking keeps film f, being made, of desktop's library to play from its computer on its network, as Stream keeps a film to play from the storage: its record, as a film kept to stream, and its lines so far, none before its first are made; then play runs with its key on the engine's goroutine. failed hears why it could not be kept.
func (s *Session) streamMaking(desktop, library string, f sync.Film, play func(key string), failed func(error)) {
	key := store.Key(desktop, f.ID)
	c := s.Client(desktop)
	rec := store.Film{Desktop: desktop, Library: library, Film: f, Added: s.cfg.Now(), Streamed: true}
	for _, lib := range s.Shelf(desktop).Listing.Libraries {
		if slices.ContainsFunc(lib.Films, func(listed sync.Film) bool { return listed.ID == f.ID }) {
			rec.LibraryID = lib.ID
		}
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, streamFor)
		defer cancel()
		cues, err := makingCues(ctx, c, f.ID)
		if err == nil {
			err = s.st.SaveCues(desktop, f.ID, cues)
		}
		if err == nil {
			err = s.st.SaveFilm(rec)
		}
		s.cx.Post(func() {
			if err != nil {
				failed(err)
				return
			}
			if _, ok := s.Film(key); !ok {
				s.films.Set(append([]store.Film{rec}, s.films.GetUntracked()...))
			}
			play(key)
		})
	})
}

// makingCues are film id's lines so far, none before its first are made, which its computer answers not found.
func makingCues(ctx context.Context, c *sync.Client, id string) ([]sync.Cue, error) {
	cues, err := c.Subtitles(ctx, id)
	if e, ok := err.(*sync.Error); ok && e.Code == sync.CodeNotFound {
		return []sync.Cue{}, nil
	}
	return cues, err
}

// Watcher is the player watching a film being made (WatchMaking).
type Watcher struct {
	s      *Session
	key    string
	at     atomic.Int64
	claims chan struct{}
	cancel context.CancelFunc
	// updated is when the film's lines last changed, as the phone last read them; changed is told once it read them again. Engine goroutine only.
	updated time.Time
	changed func()
}

// WatchMaking follows the film kept under key while the player shows it, from at, when it is a film being made kept to stream on its computer's network: it tells the computer where it is watched, at once and every sync.WatchEvery (Watcher.Move), and keeps what the computer answers, changed being told on the engine's goroutine once the film's lines were read again. It is nil for any other film. Stop it as the player closes.
func (s *Session) WatchMaking(key string, at time.Duration, changed func()) *Watcher {
	f, ok := s.Film(key)
	if !ok || !f.Streamed || !f.Film.Making || !s.OnNetwork(f.Desktop) || !s.Queue(f.Desktop).Can(sync.OpFilmWatch) {
		return nil
	}
	if old, ok := s.making.watchers[key]; ok {
		old.Stop()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	w := &Watcher{s: s, key: key, claims: make(chan struct{}, 1), cancel: cancel, updated: f.Film.Updated, changed: changed}
	w.at.Store(int64(at))
	w.claims <- struct{}{}
	s.making.watchers[key] = w
	c, id := s.Client(f.Desktop), f.Film.ID
	s.work.Go(func() {
		tick := time.NewTicker(cmp.Or(s.cfg.WatchEvery, sync.WatchEvery))
		defer tick.Stop()
		for {
			claim := false
			select {
			case <-ctx.Done():
				return
			case <-w.claims:
				claim = true
			case <-tick.C:
			}
			call, done := context.WithTimeout(ctx, sync.WatchEvery)
			got, err := c.WatchFilm(call, id, time.Duration(w.at.Load()), claim)
			done()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				// A film no longer listed, removed or failed on the computer, is watched no more; one the computer did not answer for is asked again at the next turn.
				if e, ok := err.(*sync.Error); ok && e.Code == sync.CodeNotFound {
					s.cx.Post(w.Stop)
					return
				}
				continue
			}
			s.cx.Post(func() { w.took(got) })
			if !got.Making {
				return
			}
		}
	})
	return w
}

// Move says the film is watched at at; claim, as its user opened, played, held or moved it, tells the computer at once. Engine goroutine only.
func (w *Watcher) Move(at time.Duration, claim bool) {
	if w == nil {
		return
	}
	w.at.Store(int64(at))
	if claim {
		select {
		case w.claims <- struct{}{}:
		default:
		}
	}
}

// Stop stops telling the computer the film is watched. Engine goroutine only.
func (w *Watcher) Stop() {
	if w == nil {
		return
	}
	w.cancel()
	if w.s.making.watchers[w.key] == w {
		delete(w.s.making.watchers, w.key)
	}
}

// took keeps how far the film came, as its computer answered: what plays of it, its lines read again once they changed, and, once it is done, its record as any film kept to stream. Engine goroutine only.
func (w *Watcher) took(got sync.FilmWatching) {
	s := w.s
	at := maps.Clone(s.making.at.GetUntracked())
	at[w.key] = got
	s.making.at.Set(at)
	f, ok := s.Film(w.key)
	if !ok {
		return
	}
	if got.Making && !got.Updated.After(w.updated) {
		return
	}
	w.updated = got.Updated
	c := s.Client(f.Desktop)
	if c == nil {
		return
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, streamFor)
		defer cancel()
		cues, err := makingCues(ctx, c, f.Film.ID)
		if err == nil {
			err = s.st.SaveCues(f.Desktop, f.Film.ID, cues)
		}
		s.cx.Post(func() {
			if err != nil {
				return
			}
			if cur, ok := s.Film(w.key); ok && !got.Making && cur.Film.Making {
				cur.Film.Making, cur.Film.Updated = false, got.Updated
				s.replace(cur)
				s.saveFilm(cur)
			}
			s.OpenMarks(w.key)
			if w.changed != nil {
				w.changed()
			}
		})
	})
}
