package session

import (
	"context"
	"maps"
	"strconv"
	"strings"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// Shelf is what a paired computer last listed for the Library (catalog.Load): its listing once it answered, whether it is being read, and why its last read failed, which leaves no listing.
type Shelf struct {
	Listing catalog.Listing
	// Listed is a computer that answered since the app started or paired with it.
	Listed  bool
	Loading bool
	Err     error
	// Relayed is a listing the computer gave through the user's storage away from its network (relay_films.go), not by answering itself.
	Relayed bool
}

// Waiting reports whether the shelf has nothing to show while it is read: the first time, or again after a failure.
func (sh Shelf) Waiting() bool { return sh.Loading && (!sh.Listed || sh.Err != nil) }

// Failed reports whether the last read failed and none runs.
func (sh Shelf) Failed() bool { return sh.Err != nil && !sh.Loading }

// listFor is how long a computer has to list its films: one asleep or out of reach says so after it rather than leaving the Library loading.
const listFor = 30 * time.Second

// listing is a read of a computer's shelf under way: its cancel, and the generation that tells it from a later read.
type listing struct {
	cancel context.CancelFunc
	gen    uint64
}

// Shelf is computer id's shelf.
func (s *Session) Shelf(id string) Shelf { return s.shelves.Get()[id] }

// Listing reports whether any computer's shelf is being read.
func (s *Session) Listing() bool {
	for _, sh := range s.shelves.Get() {
		if sh.Loading {
			return true
		}
	}
	return false
}

// Refresh reads every paired computer's shelf again.
func (s *Session) Refresh() {
	for _, d := range s.desktops.GetUntracked() {
		s.RefreshDesktop(d.ID)
	}
}

// RefreshDesktop reads computer id's shelf and its subscriptions again, in place of a read under way, keeping what it last listed until the answer comes.
func (s *Session) RefreshDesktop(id string) { s.refreshDesktop(id, false) }

// catchUp reads computer id's shelf again as RefreshDesktop does, for the phone's own sake rather than its user's, as it begins following the computer's events: one that fails keeps what the shelf lists, as the computer's reach is the follower's to tell; a shelf listing nothing yet takes the failure as RefreshDesktop's does.
func (s *Session) catchUp(id string) { s.refreshDesktop(id, true) }

func (s *Session) refreshDesktop(id string, catchUp bool) {
	d, ok := s.Desktop(id)
	c := s.Client(id)
	if !ok || c == nil {
		return
	}
	if l, running := s.listings[id]; running {
		l.cancel()
	}
	s.listGen++
	gen := s.listGen
	ctx, cancel := context.WithTimeout(context.Background(), listFor)
	s.listings[id] = listing{cancel: cancel, gen: gen}
	s.setShelf(id, func(sh Shelf) Shelf { sh.Loading = true; return sh })
	s.readSubscriptions(id)
	s.work.Go(func() {
		defer cancel()
		l, err := catalog.Load(ctx, c, d.DeviceID)
		s.cx.Post(func() {
			if cur, ok := s.listings[id]; !ok || cur.gen != gen {
				return
			}
			delete(s.listings, id)
			if err != nil {
				if sh := s.shelves.GetUntracked()[id]; catchUp && sh.Listed {
					s.setShelf(id, func(sh Shelf) Shelf { sh.Loading = false; return sh })
					return
				}
				// Away from its network, the films it put in the storage stand in for its answer.
				if sh, ok := s.relayShelf(id); ok && ProblemOf(err) == Unreachable {
					s.setShelf(id, func(Shelf) Shelf { return sh })
					s.keepListed(id, sh.Listing)
					return
				}
				s.setShelf(id, func(Shelf) Shelf { return Shelf{Err: err} })
				return
			}
			s.setShelf(id, func(Shelf) Shelf { return Shelf{Listing: l, Listed: true} })
			s.keepListed(id, l)
		})
	})
}

// forgetShelf stops reading computer id's shelf and its subscriptions and drops them, as the computer is unpaired or paired again.
func (s *Session) forgetShelf(id string) {
	s.forgetSubscriptions(id)
	if l, running := s.listings[id]; running {
		l.cancel()
		delete(s.listings, id)
	}
	m := maps.Clone(s.shelves.GetUntracked())
	delete(m, id)
	s.shelves.Set(m)
}

// setShelf changes computer id's shelf, and gives the films of a source the lines of the films it lists of the same files (matchLines).
func (s *Session) setShelf(id string, change func(Shelf) Shelf) {
	m := maps.Clone(s.shelves.GetUntracked())
	m[id] = change(m[id])
	s.shelves.Set(m)
	s.matchLines()
}

// Computers are the paired computers with what they last listed, in the order they were paired, for catalog.Merge.
func (s *Session) Computers() []catalog.Computer {
	shelves := s.shelves.Get()
	var out []catalog.Computer
	for _, d := range s.desktops.Get() {
		out = append(out, catalog.Computer{ID: d.ID, Name: d.Name, Listing: shelves[d.ID].Listing})
	}
	return out
}

// Entries are the films of every paired computer and those on the phone, as one library (catalog.Merge).
func (s *Session) Entries() []catalog.Entry { return catalog.Merge(s.Computers(), s.films.Get()) }

// Position is where the film kept under key was left last on any device, which the player opens it at: the place the Library shows for it (Entries, catalog.Merge: the computer's and the phone's, the newer of them), or, where the computer's learning state the phone last synced (study.go) holds a newer place than that, as for a film whose computer is out of reach and lists nothing now, that place. A film of a source matched to a computer's film is left where that film was (store.Film.LinesFilm). The zero Position for a film the phone does not keep.
func (s *Session) Position(key string) catalog.Position {
	f, ok := s.Film(key)
	if !ok {
		return catalog.Position{}
	}
	lf := f.LinesFilm()
	listed := store.Key(lf.Desktop, lf.Film.ID)
	var out catalog.Position
	for _, e := range s.Entries() {
		if e.Key() == listed {
			out = e.Position
			break
		}
	}
	if lf.LibraryID == "" {
		return out
	}
	l, ok := s.studies.Get()[studyKey(lf.Desktop, lf.LibraryID)]
	if !ok {
		return out
	}
	d, _ := s.Desktop(lf.Desktop)
	// The rule is catalog.Merge's: a place kept with a time, even the beginning, stands until a later one.
	if c, ok := catalog.Positions(sync.StateChanges{Records: l.Records, Devices: l.Devices}, d.DeviceID)[lf.Film.ID]; ok && (out.When.IsZero() && out.At <= 0 || c.When.After(out.When)) {
		out = c
	}
	return out
}

// LinesKey names the lines kept on the phone as they stand: the films whose videos are whole, each as of its desktop's last change, which a search reads its lines again on, and for a film of a source that took a computer's film's lines, streamed or downloaded, that film's key, whose folder they are in (store.Film.LinesFilm) and which the Library lists it under (catalog.Merge).
func (s *Session) LinesKey() string {
	var b strings.Builder
	for _, f := range s.films.Get() {
		if f.Done() || f.Streamed || f.Lines != nil {
			lf := f.LinesFilm()
			b.WriteString(f.Key())
			b.WriteByte('@')
			b.WriteString(strconv.FormatInt(lf.Film.Updated.UnixNano(), 36))
			if f.Lines != nil {
				b.WriteByte('@')
				b.WriteString(lf.Key())
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Lines reads the lines of the films LinesKey names, by the key the Library lists each under, for a search: a film of a source that took a computer's film's lines by that film's (catalog.Merge). It runs off the engine's goroutine, as a resource's fetcher. A film whose lines cannot be read has none.
func (s *Session) Lines(ctx context.Context, key string) map[string][]sync.Cue {
	out := map[string][]sync.Cue{}
	for line := range strings.Lines(key) {
		parts := strings.Split(strings.TrimSuffix(line, "\n"), "@")
		from := parts[0]
		if len(parts) > 2 {
			from = parts[2]
		}
		desktop, film, ok := strings.Cut(from, "/")
		if !ok || ctx.Err() != nil {
			continue
		}
		if cues, ok, _ := s.st.Cues(desktop, film); ok {
			out[from] = cues
		}
	}
	return out
}
