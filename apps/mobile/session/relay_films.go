package session

import (
	"context"
	"errors"
	"maps"
	"reflect"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/download"
	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// Films through the storage (kando/packages/sync, relay_films.go; yldm-tech/Kando#219): a computer whose relay is on puts its films in the user's storage and lists them in the state.json the phone reads there away from its network (sync.RelayState.Shelf). Away from that network the Library lists them as the computer's; a film's page plays its video from its link, the system player streaming it (Stream), or downloads it by the same links as from the computer (download.Source); and a film the computer shares as its lines alone opens for its lines, its video being on the computer. Back on the network, the computer lists its films itself again.

// relayFilms is what the computers put in the storage, by computer ID, as their state.json last listed it: the shelf, and its films by the links.
type relayFilms struct {
	shelf sync.RelayShelf
	films sync.RelayFilms
}

// errNotInStorage is Stream's error for a film the storage does not hold.
var errNotInStorage = errors.New("the film is not in the storage")

// takeShelf takes the films desktop lists in its storage as the follower read them away from its network: the Library lists them as the computer's from then on. Engine goroutine only.
func (s *Session) takeShelf(desktop string, shelf sync.RelayShelf) {
	cur, had := s.remote.shelf.GetUntracked()[desktop]
	if had && reflect.DeepEqual(cur.shelf, shelf) && s.shelves.GetUntracked()[desktop].Relayed {
		return
	}
	m := maps.Clone(s.remote.shelf.GetUntracked())
	m[desktop] = relayFilms{shelf: shelf, films: sync.NewRelayFilms(shelf, nil)}
	s.remote.shelf.Set(m)
	s.remote.shelfMu.Lock()
	s.remote.shelfCopy, s.remote.shelfAway[desktop] = m, true
	s.remote.shelfMu.Unlock()
	if _, listing := s.listings[desktop]; listing {
		// A read of the computer under way says how it ends; one that fails falls back to these films (RefreshDesktop).
		return
	}
	s.setShelf(desktop, func(Shelf) Shelf { return Shelf{Listing: catalog.FromRelay(shelf), Listed: true, Relayed: true} })
}

// relayShelf is the Listing desktop last listed through its storage, false for none.
func (s *Session) relayShelf(desktop string) (Shelf, bool) {
	rf, ok := s.remote.shelf.GetUntracked()[desktop]
	if !ok {
		return Shelf{}, false
	}
	return Shelf{Listing: catalog.FromRelay(rf.shelf), Listed: true, Relayed: true}, true
}

// shelfLanded is desktop answering on its network again: its posters come from it, and the Library lists its films as it lists them itself. Engine goroutine only.
func (s *Session) shelfLanded(desktop string) {
	s.remote.shelfMu.Lock()
	delete(s.remote.shelfAway, desktop)
	s.remote.shelfMu.Unlock()
	if s.shelves.GetUntracked()[desktop].Relayed {
		s.RefreshDesktop(desktop)
	}
}

// forgetShelfFilms drops what desktop listed in its storage, as it is unpaired. Engine goroutine only.
func (s *Session) forgetShelfFilms(desktop string) {
	m := maps.Clone(s.remote.shelf.GetUntracked())
	delete(m, desktop)
	s.remote.shelf.Set(m)
	s.remote.shelfMu.Lock()
	s.remote.shelfCopy = m
	delete(s.remote.shelfAway, desktop)
	s.remote.shelfMu.Unlock()
}

// RelayFilm is film id as desktop lists it in its storage, while the phone reaches that computer through the storage, false otherwise. Read it in a binding.
func (s *Session) RelayFilm(desktop, id string) (sync.RelayFilm, bool) {
	if !s.remote.queues.Get()[desktop].Relayed {
		return sync.RelayFilm{}, false
	}
	f, ok := s.remote.shelf.Get()[desktop].films.Films[id]
	return f, ok
}

// StreamLink is the link the film kept under key streams from, out of its computer's storage, "" when the storage holds no video of it. Read it in a binding.
func (s *Session) StreamLink(key string) string {
	f, ok := s.Film(key)
	if !ok {
		return ""
	}
	return s.remote.shelf.Get()[f.Desktop].films.VideoLink(f.Film.ID)
}

// relaySource is desktop's films in its storage, for downloading film id away from its network: false while the phone reaches the computer on its network, or the storage holds no video of the film. Engine goroutine only.
func (s *Session) relaySource(desktop, id string) (download.Source, bool) {
	if !s.remote.queues.GetUntracked()[desktop].Relayed {
		return nil, false
	}
	rf := s.remote.shelf.GetUntracked()[desktop]
	if rf.films.VideoLink(id) == "" {
		return nil, false
	}
	return rf.films, true
}

// relayPoster is desktop's films in its storage while the phone reaches it through the storage, for a poster's fetcher off the engine's goroutine; false otherwise, or for a film without a poster there.
func (s *Session) relayPoster(desktop, id string) (sync.RelayFilms, bool) {
	s.remote.shelfMu.Lock()
	defer s.remote.shelfMu.Unlock()
	rf, ok := s.remote.shelfCopy[desktop]
	if !ok || !s.remote.shelfAway[desktop] || rf.films.Films[id].Poster == "" {
		return sync.RelayFilms{}, false
	}
	return rf.films, true
}

// streamFor is how long Stream waits for the storage to give a film's lines.
const streamFor = listFor

// Stream keeps film f of desktop's library to play from the user's storage away from its computer: its record, as a film kept to stream (store.Film.Streamed), and its lines, read by their link; then play runs with its key on the engine's goroutine, and the player streams the video from its link (StreamLink), or, for a film whose video is not in the storage, shows its lines alone. A film kept already plays as it is. failed hears why the lines could not be read.
func (s *Session) Stream(desktop, library string, f sync.Film, play func(key string), failed func(error)) {
	key := store.Key(desktop, f.ID)
	if _, ok := s.Film(key); ok {
		play(key)
		return
	}
	rf, ok := s.remote.shelf.GetUntracked()[desktop]
	listed, listedOK := rf.films.Films[f.ID]
	if !ok || !listedOK {
		failed(errNotInStorage)
		return
	}
	rec := store.Film{Desktop: desktop, Library: library, LibraryID: listed.Library, Film: f, Added: s.cfg.Now(), Streamed: true}
	films := rf.films
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, streamFor)
		defer cancel()
		cues, err := films.Subtitles(ctx, f.ID)
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
