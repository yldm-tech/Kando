// Package session is the phone app's state: the paired desktops, the films kept on the phone and the downloads under way, held in signals on the engine's goroutine, which the screens read in their bindings. Disk and network work runs on goroutines of its own (store, download) and comes back through the scope's Post.
package session

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	gosync "sync"
	"sync/atomic"
	"syscall"
	"time"

	"kando/apps/mobile/download"
	"kando/apps/mobile/langs"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/store"
	"kando/apps/mobile/study"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// State is where a download stands.
type State uint8

const (
	// Running is a download under way.
	Running State = iota
	// Paused is one the user paused, or the app stopped as it went.
	Paused
	// Failed is one that stopped on its own, for Problem.
	Failed
)

// Problem is why a request to a desktop or a download failed, as the screens tell it.
type Problem uint8

const (
	NoProblem Problem = iota
	// Unreachable is a desktop the phone cannot reach: off, asleep, on another network, or Phone Sync off.
	Unreachable
	// Unpaired is a desktop that no longer takes the phone's token: it revoked the phone.
	Unpaired
	// Gone is a film or library the desktop no longer serves.
	Gone
	// NoSpace is a phone whose storage is full.
	NoSpace
	// Certificate is a desktop answering with another certificate than the one paired with.
	Certificate
)

// ProblemOf is why err happened, as the screens tell it.
func ProblemOf(err error) Problem {
	var e *sync.Error
	switch {
	case err == nil:
		return NoProblem
	case errors.As(err, &e) && e.Status == http.StatusUnauthorized:
		return Unpaired
	case errors.As(err, &e) && e.Status == http.StatusNotFound:
		return Gone
	case errors.Is(err, sync.ErrFingerprint):
		return Certificate
	case errors.Is(err, syscall.ENOSPC):
		return NoSpace
	}
	return Unreachable
}

// Transfer is a download of a film's files: where it stands, how many of the video's bytes it has, and why it stopped when it failed.
type Transfer struct {
	State       State
	Done, Total int64
	Problem     Problem
}

// Fraction is how much of the video is down, from 0 to 1.
func (t Transfer) Fraction() float32 {
	if t.Total <= 0 {
		return 0
	}
	return min(float32(t.Done)/float32(t.Total), 1)
}

// Config is what the session starts with.
type Config struct {
	// Dir is the app's data folder, where the store keeps its files.
	Dir string
	// Device and Platform are what pairing tells a desktop the phone is (sync.PairRequest).
	Device, Platform string
	// Now is the clock pairings and downloads are dated by; time.Now when nil.
	Now func() time.Time
	// RelayEvery is how often the phone reads a computer's state.json through the storage relay while away from its network (relay.go), sync.RelayPoll when 0. Tests shorten it.
	RelayEvery time.Duration
}

// Session is the app's state. Its methods belong to the engine's goroutine.
type Session struct {
	cx        *core.Scope
	st        *store.Store
	cfg       Config
	loaded    *core.Signal[bool]
	desktops  *core.Signal[[]store.Desktop]
	films     *core.Signal[[]store.Film]
	transfers *core.Signal[map[string]Transfer]
	used      *core.Signal[int64]
	prefs     *core.Signal[store.Prefs]
	// shelves are what the paired computers last listed, by ID, and listings the reads of them under way, each told from a later one by listGen (Shelf).
	shelves  *core.Signal[map[string]Shelf]
	listings map[string]listing
	listGen  uint64
	jobs     map[string]*job
	// following are the films Follow is bringing up to date, by key, so a film opened again meanwhile does not ask its desktop twice.
	following map[string]*follow
	clients   map[string]*sync.Client
	// work counts the goroutines that read and write the store, which the session waits for as it goes, so none outlives it.
	work  gosync.WaitGroup
	awake func()
	// posters are the remote films' posters fetched so far, by film key, which fetchers read and write off the engine's goroutine.
	postersMu gosync.Mutex
	posters   map[string][]byte
	// secrets is the platform's secure store, which keeps the desktops' device tokens.
	secrets core.Secrets
	// saveMu orders the saves of the desktops, which run on goroutines of their own: saving is the generation setDesktops last gave out, saved the one last written and savedIDs the desktops it kept, so an older list never overwrites a newer one and an unpaired desktop's token is deleted.
	saveMu   gosync.Mutex
	saving   uint64
	saved    uint64
	savedIDs []string
	// filmMu orders the writes of the films' records, which run on goroutines of their own: filmSaves holds, by key, what is still to be written of a film while a goroutine writes it, so a film's record is written one save at a time, the latest last, and a save that fails is tried again until it is written, a later one takes its place or the session ends (closed).
	filmMu    gosync.Mutex
	filmSaves map[string]*filmSave
	closed    chan struct{}
	// ctx ends with the session, so the learning state's syncs, lookups and explanations on their way do not hold the app up as it closes (study.go).
	ctx    context.Context
	cancel context.CancelFunc
	// prefsMu orders the saves of the preferences as saveMu does the desktops': prefsSaving is the generation SetPrefs last gave out, prefsSaved the one last written.
	prefsMu                 gosync.Mutex
	prefsSaving, prefsSaved uint64
	// studies are the libraries' learning state, by desktop and library (studyKey), the one store of it the player and the Learn tab both read and change: all read from disk as the session starts (newLearn), or one the first time a film of it opens before then (study.go); studyLoading the ones being read, studyRuns the syncs running, finding the films whose library is being asked for. studyMu orders their saves as prefsMu does the preferences', by library.
	studies      *core.Signal[map[string]study.Library]
	studyLoading map[string]bool
	studyRuns    map[string]*studyRun
	finding      map[string]bool
	studyMu      gosync.Mutex
	studySaving  map[string]uint64
	studySaved   map[string]uint64
	// lookups are the words looked up through the desktops, read with the store, and lookupRev counts their changes, for bindings to read; lookupMu orders their saves.
	lookups                   map[string]store.Lookup
	lookupRev                 *core.Signal[int]
	lookupMu                  gosync.Mutex
	lookupSaving, lookupSaved uint64
	// remote follows the computers' queues (queue.go).
	remote *remote
	// learn is the learner's study (learning.go).
	learn *learn
	// subs are the computers' subscriptions (subscriptions.go).
	subs *subscriptions
	// settle is the conflicts being resolved (conflicts.go).
	settle *settle
}

// filmSave is what is still to be written of a film's record: next, once pending; or its folder deleted, once removed.
type filmSave struct {
	next             store.Film
	pending, removed bool
}

// The waits between the tries of a film's record that could not be written: the first, doubled each time up to the last.
const (
	filmRetryFirst = 100 * time.Millisecond
	filmRetryLast  = 5 * time.Second
)

// job is a download running on a goroutine of its own.
type job struct {
	cancel context.CancelFunc
	// remove deletes the film once the goroutine is done with its folder, as Delete asks while it runs.
	remove atomic.Bool
	// paused is a job Pause or Unpair stopped, whose goroutine may still be finishing; again starts the download anew once it has, as Resume asks meanwhile. Both belong to the engine's goroutine.
	paused, again bool
}

// follow is a check Follow runs on a goroutine of its own. Its fields belong to the engine's goroutine.
type follow struct {
	cancel context.CancelFunc
	// changed are the calls to make once the film took its desktop's changes: one for each player that asked while the check ran.
	changed []func()
}

// New is the session of the app under cx, which reads the store in cfg.Dir off the engine's goroutine: Loaded turns true once it has.
func New(cx *core.Scope, cfg Config) *Session {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	secrets := cx.Secrets()
	// Downloads go on with the screen locked, after the first unlock; ThisDeviceOnly keeps the tokens out of backups and off other devices.
	secrets.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
	s := &Session{
		cx: cx, st: store.Open(cfg.Dir), cfg: cfg, secrets: secrets,
		loaded:       core.NewSignal(cx, false),
		desktops:     core.NewSignalFunc[[]store.Desktop](cx, nil, nil),
		films:        core.NewSignalFunc[[]store.Film](cx, nil, nil),
		transfers:    core.NewSignalFunc(cx, map[string]Transfer{}, nil),
		used:         core.NewSignal(cx, int64(0)),
		prefs:        core.NewSignalFunc(cx, store.Prefs{}, nil),
		shelves:      core.NewSignalFunc(cx, map[string]Shelf{}, nil),
		listings:     map[string]listing{},
		jobs:         map[string]*job{},
		following:    map[string]*follow{},
		clients:      map[string]*sync.Client{},
		posters:      map[string][]byte{},
		filmSaves:    map[string]*filmSave{},
		closed:       make(chan struct{}),
		remote:       newRemote(cx),
		subs:         newSubscriptions(cx),
		studies:      core.NewSignalFunc(cx, map[string]study.Library{}, nil),
		studyLoading: map[string]bool{}, studyRuns: map[string]*studyRun{}, finding: map[string]bool{},
		studySaving: map[string]uint64{}, studySaved: map[string]uint64{},
		lookups: map[string]store.Lookup{}, lookupRev: core.NewSignal(cx, 0),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.learn = newLearn(s)
	s.settle = newSettle(cx)
	cx.Cleanup(func() {
		close(s.closed)
		s.cancel()
		s.stopFollowing()
		for _, j := range s.jobs {
			j.cancel()
		}
		for _, fw := range s.following {
			fw.cancel()
		}
		for _, l := range s.listings {
			l.cancel()
		}
		for _, c := range s.clients {
			c.Close()
		}
		if s.awake != nil {
			s.awake()
		}
		s.work.Wait()
	})
	s.work.Go(func() {
		desktops, _ := s.st.Desktops()
		s.saveMu.Lock()
		for i, d := range desktops {
			// A token the secure store lost leaves the desktop paired without one, which it refuses as it would a revoked phone, so the phone pairs again.
			desktops[i].Token, _, _ = s.secrets.Get(store.TokenAccount(d.ID))
			s.savedIDs = append(s.savedIDs, d.ID)
		}
		s.saveMu.Unlock()
		films, _ := s.st.Films()
		used, _ := s.st.Used()
		prefs, _ := s.st.Prefs()
		lookups, _ := s.st.Lookups()
		links, _ := s.st.Links()
		// A phone that paired or downloaded before the introduction existed has been introduced already.
		migrated := !prefs.Onboarded && (len(desktops) > 0 || len(films) > 0)
		prefs.Onboarded = prefs.Onboarded || migrated
		parts := map[string]int64{}
		for _, f := range films {
			if !f.Done() {
				parts[f.Key()] = partSize(s.st, f)
			}
		}
		for _, f := range films {
			// A film kept to stream whose download was never asked for is not one paused.
			if f.Streamed && parts[f.Key()] == 0 {
				delete(parts, f.Key())
			}
		}
		cx.Post(func() {
			s.desktops.Set(desktops)
			s.films.Set(films)
			s.used.Set(used)
			// A word looked up before the store was read stays.
			maps.Copy(lookups, s.lookups)
			s.lookups = lookups
			s.lookupRev.Update(func(n int) int { return n + 1 })
			if migrated {
				s.SetPrefs(prefs)
			} else {
				s.prefs.Set(prefs)
			}
			t := map[string]Transfer{}
			for key, done := range parts {
				// A download the app was closed during waits to be resumed.
				f, _ := s.Film(key)
				t[key] = Transfer{State: Paused, Done: done, Total: f.Film.VideoSize}
			}
			s.transfers.Set(t)
			s.loadLinks(links)
			for _, d := range desktops {
				s.follow(d.ID)
			}
			s.loaded.Set(true)
			s.Refresh()
		})
	})
	return s
}

// Loaded reports whether the store has been read.
func (s *Session) Loaded() bool { return s.loaded.Get() }

// Prefs are the user's choices on this phone.
func (s *Session) Prefs() store.Prefs { return s.prefs.Get() }

// SetPrefs keeps p as the user's choices.
func (s *Session) SetPrefs(p store.Prefs) {
	s.prefs.Set(p)
	s.prefsMu.Lock()
	s.prefsSaving++
	gen := s.prefsSaving
	s.prefsMu.Unlock()
	s.work.Go(func() {
		s.prefsMu.Lock()
		defer s.prefsMu.Unlock()
		if gen > s.prefsSaved && s.st.SavePrefs(p) == nil {
			s.prefsSaved = gen
		}
	})
}

// Target is the language the user wants subtitles translated into: their choice, else the phone's own language where the computer translates into it (langs.DefaultTarget).
func (s *Session) Target() string {
	if t := s.prefs.Get().Target; t != "" {
		return t
	}
	return langs.DefaultTarget(s.cx.Env().Locale())
}

// Platform is the platform pairing names (sync.PlatformIOS, sync.PlatformAndroid).
func (s *Session) Platform() string { return s.cfg.Platform }

// Device is the name pairing gives the phone.
func (s *Session) Device() string { return s.cfg.Device }

// Desktops are the paired desktops, in the order they were paired.
func (s *Session) Desktops() []store.Desktop { return s.desktops.Get() }

// Desktop is the paired desktop id.
func (s *Session) Desktop(id string) (store.Desktop, bool) {
	i := slices.IndexFunc(s.desktops.Get(), func(d store.Desktop) bool { return d.ID == id })
	if i < 0 {
		return store.Desktop{}, false
	}
	return s.desktops.Get()[i], true
}

// Client is the client of desktop id, nil for one not paired. It may be used from any goroutine.
func (s *Session) Client(id string) *sync.Client {
	if c, ok := s.clients[id]; ok {
		return c
	}
	d, ok := s.Desktop(id)
	if !ok {
		return nil
	}
	c := sync.NewClient(d.Addr, d.Fingerprint, d.Token)
	s.clients[id] = c
	return c
}

// AddDesktop keeps d as paired, in place of an earlier pairing with the same desktop.
func (s *Session) AddDesktop(d store.Desktop) {
	if d.Paired.IsZero() {
		d.Paired = s.cfg.Now()
	}
	if c, ok := s.clients[d.ID]; ok {
		c.Close()
		delete(s.clients, d.ID)
	}
	s.forgetShelf(d.ID)
	list := slices.Clone(s.desktops.Get())
	if i := slices.IndexFunc(list, func(x store.Desktop) bool { return x.ID == d.ID }); i >= 0 {
		list[i] = d
	} else {
		list = append(list, d)
	}
	s.setDesktops(list)
	// Paired again, a computer that revoked the phone before shows the page that says so if it does again.
	delete(s.learn.dismissed, d.ID)
	// The computer takes the language the phone translates into, once the phone has one.
	if p := s.prefs.Get(); p.Target != "" && !slices.Contains(p.TargetPending, d.ID) {
		p.TargetPending = append(slices.Clone(p.TargetPending), d.ID)
		s.SetPrefs(p)
	}
	s.RefreshDesktop(d.ID)
	s.follow(d.ID)
}

// Unpair forgets desktop id and what the phone kept of its learning state, and stops its downloads; the films kept from it stay.
func (s *Session) Unpair(id string) {
	for key, j := range s.jobs {
		if f, ok := s.Film(key); ok && f.Desktop == id {
			j.paused, j.again = true, false
			j.cancel()
			s.setTransfer(key, func(t Transfer) Transfer { t.State = Paused; return t })
		}
	}
	s.forgetQueue(id)
	if c, ok := s.clients[id]; ok {
		c.Close()
		delete(s.clients, id)
	}
	s.forgetShelf(id)
	s.setDesktops(slices.DeleteFunc(slices.Clone(s.desktops.Get()), func(d store.Desktop) bool { return d.ID == id }))
	s.learnForget(id)
}

func (s *Session) setDesktops(list []store.Desktop) {
	s.desktops.Set(list)
	list = slices.Clone(list)
	s.saveMu.Lock()
	s.saving++
	gen := s.saving
	s.saveMu.Unlock()
	s.work.Go(func() { _ = s.saveDesktops(gen, list) })
}

// saveDesktops keeps list, generation gen of the desktops, unless a later one is kept already: the tokens in the secure store first, then the desktops without them, then the tokens of the desktops no longer in it deleted.
func (s *Session) saveDesktops(gen uint64, list []store.Desktop) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if gen <= s.saved {
		return nil
	}
	ids := make([]string, len(list))
	for i, d := range list {
		ids[i] = d.ID
		if err := s.secrets.Set(store.TokenAccount(d.ID), d.Token); err != nil {
			return err
		}
	}
	if err := s.st.SaveDesktops(list); err != nil {
		return err
	}
	for _, id := range s.savedIDs {
		if !slices.Contains(ids, id) {
			_ = s.secrets.Delete(store.TokenAccount(id))
		}
	}
	s.saved, s.savedIDs = gen, ids
	return nil
}

// Films are the films kept on the phone, downloading ones too, the latest added first.
func (s *Session) Films() []store.Film { return s.films.Get() }

// Film is the film kept under key (store.Key).
func (s *Session) Film(key string) (store.Film, bool) {
	list := s.films.Get()
	i := slices.IndexFunc(list, func(f store.Film) bool { return f.Key() == key })
	if i < 0 {
		return store.Film{}, false
	}
	return list[i], true
}

// Transfer is the download of the film under key, false when none is under way, paused or failed.
func (s *Session) Transfer(key string) (Transfer, bool) {
	t, ok := s.transfers.Get()[key]
	return t, ok
}

// Used is how many bytes the films kept on the phone take.
func (s *Session) Used() int64 { return s.used.Get() }

// Store is the store the session reads and writes, for the player's files.
func (s *Session) Store() *store.Store { return s.st }

// Download starts downloading f of desktop's library, or goes on with it.
func (s *Session) Download(desktop, library string, f sync.Film) {
	key := store.Key(desktop, f.ID)
	if kept, ok := s.Film(key); ok {
		s.start(kept)
		return
	}
	rec := store.Film{Desktop: desktop, Library: library, Film: f, Added: s.cfg.Now()}
	if listed, ok := s.remote.shelf.GetUntracked()[desktop].films.Films[f.ID]; ok && s.remote.queues.GetUntracked()[desktop].Relayed {
		// Its library's ID comes with it from the storage, so its learning state is known without asking the computer.
		rec.LibraryID = listed.Library
	}
	s.films.Set(append([]store.Film{rec}, s.films.Get()...))
	s.start(rec)
}

// followFor is how long Follow waits for a desktop, which may be off or out of reach, as the phone is used away from it.
const followFor = 15 * time.Second

// Follow brings the film kept under key, a downloaded one, up to date with its desktop when the desktop can be reached (download.Follow): one translated again or proofread there since takes the desktop's subtitles and record, and changed runs on the engine's goroutine once they are kept, so a player showing the film reads its lines again. A film whose desktop is out of reach, or not paired any more, stays as it is, without a word: it plays as it was downloaded.
func (s *Session) Follow(key string, changed func()) {
	f, ok := s.Film(key)
	if !ok || !f.Done() {
		return
	}
	if fw, running := s.following[key]; running {
		fw.changed = append(fw.changed, changed)
		return
	}
	c := s.Client(f.Desktop)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), followFor)
	fw := &follow{cancel: cancel, changed: []func(){changed}}
	s.following[key] = fw
	s.work.Go(func() {
		defer cancel()
		listed, cues, updated, _ := download.Follow(ctx, c, f)
		s.cx.Post(func() {
			cur, ok := s.followed(key, listed)
			if !updated || !ok {
				delete(s.following, key)
				return
			}
			// The subtitles first: a record kept with the desktop's date says they are the desktop's, so an app closed between the two asks for them again.
			s.work.Go(func() {
				err := s.st.SaveCues(cur.Desktop, cur.Film.ID, cues)
				s.cx.Post(func() {
					delete(s.following, key)
					if err != nil {
						return
					}
					// Taken again as it stands now, so a position kept while the subtitles were written stays, and kept after the saves of the position before it.
					if cur, ok := s.followed(key, listed); ok {
						s.replace(cur)
						s.saveFilm(cur)
						for _, fn := range fw.changed {
							fn()
						}
					}
				})
			})
		})
	})
}

// followed is the film kept under key as it stands now, with its desktop's record listed in place of its own but its video's size and poster, which are the phone's files; false for a film deleted meanwhile, or downloading again, which Follow does not touch.
func (s *Session) followed(key string, listed sync.Film) (store.Film, bool) {
	cur, ok := s.Film(key)
	if !ok || !cur.Done() {
		return store.Film{}, false
	}
	listed.VideoSize, listed.Poster = cur.Film.VideoSize, cur.Film.Poster
	cur.Film = listed
	return cur, true
}

// Pause stops the download of key, keeping what it has, which Resume goes on from.
func (s *Session) Pause(key string) {
	if j, ok := s.jobs[key]; ok {
		j.paused, j.again = true, false
		j.cancel()
		s.setTransfer(key, func(t Transfer) Transfer { t.State = Paused; return t })
	}
}

// Resume goes on with the download of key.
func (s *Session) Resume(key string) {
	if f, ok := s.Film(key); ok {
		s.start(f)
	}
}

// Delete removes the film under key from the phone, stopping its download first.
func (s *Session) Delete(key string) {
	if j, ok := s.jobs[key]; ok {
		j.remove.Store(true)
		j.cancel()
		return
	}
	s.remove(key)
}

// SavePosition keeps where the film under key was left playing, and that it was left there now, even where it was left there before: a film played from the beginning again is newer on the phone than where a computer left it (catalog.Merge). Its computer takes it through the film's library's learning state (yldm-tech/Kando#217), at once or once it can be reached, so the computer and the other devices go on from there; a film whose library is not known yet (Studyable) keeps it on the phone alone.
func (s *Session) SavePosition(key string, at time.Duration) { s.savePosition(key, at, true) }

// KeepPosition is SavePosition for a film that plays on: the computer takes the position with the next sync of the film's library rather than at once, so a film playing does not call the computer every few seconds.
func (s *Session) KeepPosition(key string, at time.Duration) { s.savePosition(key, at, false) }

// savePosition keeps where the film under key was left, and syncs its library when send is set.
func (s *Session) savePosition(key string, at time.Duration, send bool) {
	f, ok := s.Film(key)
	if !ok {
		return
	}
	f.PositionMS, f.PositionAt = at.Milliseconds(), s.cfg.Now()
	s.replace(f)
	s.saveFilm(f)
	if f.LibraryID == "" {
		return
	}
	// A library still being read from disk is not written over: the player's next save sends the position.
	k := studyKey(f.Desktop, f.LibraryID)
	if _, ok := s.studies.GetUntracked()[k]; !ok && (!s.learn.ready.GetUntracked() || s.studyLoading[k]) {
		return
	}
	s.setStudy(f.Desktop, f.LibraryID, study.Change(s.studies.GetUntracked()[k], study.Position(f.Film.ID, at, f.PositionAt)))
	if send {
		s.syncStudy(f.Desktop, f.LibraryID)
	}
}

// saveFilm keeps f's record on disk after the saves of it handed before, the latest last, trying again until it is written (filmSaves).
func (s *Session) saveFilm(f store.Film) {
	key := f.Key()
	s.filmMu.Lock()
	defer s.filmMu.Unlock()
	if fs, busy := s.filmSaves[key]; busy {
		fs.next, fs.pending, fs.removed = f, true, false
		return
	}
	s.filmSaves[key] = &filmSave{next: f, pending: true}
	s.work.Go(func() { s.writeFilm(key) })
}

// writeFilm writes what is to be written of the film under key, one save at a time, until nothing is left, on a goroutine of its own.
func (s *Session) writeFilm(key string) {
	wait := filmRetryFirst
	for {
		s.filmMu.Lock()
		fs := s.filmSaves[key]
		f, removed := fs.next, fs.removed
		if !fs.pending && !removed {
			delete(s.filmSaves, key)
			s.filmMu.Unlock()
			return
		}
		fs.pending = false
		if removed {
			delete(s.filmSaves, key)
		}
		s.filmMu.Unlock()
		if removed {
			s.deleteFilm(f)
			return
		}
		if err := s.st.SaveFilm(f); err == nil {
			wait = filmRetryFirst
			continue
		}
		s.filmMu.Lock()
		superseded := fs.pending || fs.removed
		fs.pending = true
		if !superseded {
			fs.next = f
		}
		s.filmMu.Unlock()
		if superseded {
			// A later save or the film's removal is written at once instead.
			continue
		}
		select {
		case <-s.closed:
			s.filmMu.Lock()
			delete(s.filmSaves, key)
			s.filmMu.Unlock()
			return
		case <-time.After(wait):
		}
		wait = min(2*wait, filmRetryLast)
	}
}

// Poster is the poster of a remote film, fetched once and kept for the session: it may be called from any goroutine, as a resource's fetcher.
func (s *Session) Poster(ctx context.Context, c *sync.Client, desktop, id string) ([]byte, error) {
	key := store.Key(desktop, id)
	s.postersMu.Lock()
	data, ok := s.posters[key]
	s.postersMu.Unlock()
	if ok {
		return data, nil
	}
	var err error
	// Away from its computer's network, the poster comes from the user's storage.
	if films, ok := s.relayPoster(desktop, id); ok {
		data, _, err = films.Poster(ctx, id)
	} else {
		data, _, err = c.Poster(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	s.postersMu.Lock()
	s.posters[key] = data
	s.postersMu.Unlock()
	return data, nil
}

// KnownPoster is the poster of the film under key that Poster has fetched this session, nil for none: it may be called from any goroutine, as the widgets read it (kando/apps/mobile/widgets).
func (s *Session) KnownPoster(key string) []byte {
	s.postersMu.Lock()
	defer s.postersMu.Unlock()
	return s.posters[key]
}

func (s *Session) start(f store.Film) {
	key := f.Key()
	if f.Streamed && !f.Done() {
		// Its download is asked for now: it is a film being downloaded from here on, which stops as one paused.
		f.Streamed = false
		s.replace(f)
	}
	if j, running := s.jobs[key]; running || f.Done() {
		if running && j.paused && !j.remove.Load() {
			// Paused, but still finishing: the download starts again once the goroutine is done with the part file.
			j.again = true
			s.setTransfer(key, func(t Transfer) Transfer { t.State, t.Problem = Running, NoProblem; return t })
		}
		return
	}
	// Away from its computer's network, the film comes from the user's storage, by the links its computer listed there.
	src, ok := s.relaySource(f.Desktop, f.Film.ID)
	if !ok {
		c := s.Client(f.Desktop)
		if c == nil {
			s.setTransfer(key, func(t Transfer) Transfer { t.State, t.Problem = Failed, Unpaired; return t })
			return
		}
		src = c
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{cancel: cancel}
	s.jobs[key] = j
	s.setTransfer(key, func(t Transfer) Transfer {
		return Transfer{State: Running, Done: t.Done, Total: f.Film.VideoSize}
	})
	s.keepAwake()
	// Progress comes back one Post at a time: the goroutine keeps the latest count, and posts again only once the last post has read it.
	var done, total atomic.Int64
	var posted atomic.Bool
	report := func() {
		posted.Store(false)
		if s.jobs[key] == j {
			s.setTransfer(key, func(t Transfer) Transfer { t.Done, t.Total = done.Load(), total.Load(); return t })
		}
	}
	s.work.Go(func() {
		got, err := download.Film(ctx, src, s.st, f, func(d, n int64) {
			done.Store(d)
			total.Store(n)
			if posted.CompareAndSwap(false, true) {
				s.cx.Post(report)
			}
		})
		stopped := ctx.Err() != nil
		cancel()
		if j.remove.Load() {
			_ = s.st.DeleteFilm(f.Desktop, f.Film.ID)
		}
		used, _ := s.st.Used()
		s.cx.Post(func() { s.finished(key, j, got, err, stopped, used) })
	})
}

// finished is the end of job j of key, on the engine's goroutine.
func (s *Session) finished(key string, j *job, got store.Film, err error, stopped bool, used int64) {
	if s.jobs[key] == j {
		delete(s.jobs, key)
	}
	s.used.Set(used)
	defer s.keepAwake()
	if j.remove.Load() {
		s.dropFilm(key)
		return
	}
	s.replace(got)
	if j.again && err != nil {
		s.start(got)
		return
	}
	switch {
	case err == nil:
		s.setTransfer(key, nil)
		if got.LibraryID == "" {
			// The library's ID goes with the film, as its learning state goes by it, so lines can be saved from it away from the desktop; it is asked for once the film is done, so the download does not wait on it, and a desktop that does not answer leaves it to be asked for when the film opens (OpenStudy).
			s.findLibrary(got)
		}
	case stopped:
		s.setTransfer(key, func(t Transfer) Transfer { t.State = Paused; return t })
	default:
		s.setTransfer(key, func(t Transfer) Transfer { t.State, t.Problem = Failed, ProblemOf(err); return t })
	}
}

func (s *Session) remove(key string) {
	f, ok := s.Film(key)
	if !ok {
		return
	}
	s.dropFilm(key)
	s.filmMu.Lock()
	defer s.filmMu.Unlock()
	if fs, busy := s.filmSaves[key]; busy {
		// The folder goes once the record being written is, so the write does not make it again.
		fs.next, fs.pending, fs.removed = f, false, true
		return
	}
	s.work.Go(func() { s.deleteFilm(f) })
}

// deleteFilm removes f's folder, off the engine's goroutine.
func (s *Session) deleteFilm(f store.Film) {
	_ = s.st.DeleteFilm(f.Desktop, f.Film.ID)
	used, _ := s.st.Used()
	s.cx.Post(func() { s.used.Set(used) })
}

func (s *Session) dropFilm(key string) {
	s.films.Set(slices.DeleteFunc(slices.Clone(s.films.Get()), func(f store.Film) bool { return f.Key() == key }))
	s.setTransfer(key, nil)
}

// replace puts f in place of the record of the same key.
func (s *Session) replace(f store.Film) {
	list := slices.Clone(s.films.Get())
	if i := slices.IndexFunc(list, func(x store.Film) bool { return x.Key() == f.Key() }); i >= 0 {
		list[i] = f
		s.films.Set(list)
	}
}

// setTransfer changes the transfer of key by change, which a nil change removes.
func (s *Session) setTransfer(key string, change func(Transfer) Transfer) {
	m := maps.Clone(s.transfers.GetUntracked())
	if change == nil {
		delete(m, key)
	} else {
		m[key] = change(m[key])
	}
	s.transfers.Set(m)
}

// keepAwake keeps the screen on while a download runs, so the phone does not lock and stop it: downloads go on only while the app is in front.
// TODO(nsgo#452): let a download go on in the background, as iOS's background URLSession and Android's WorkManager or a data-sync foreground service do; NSGo's networking is Go's net/http, which the system suspends with the app https://github.com/nsgo-dev/nsgo/issues/452
func (s *Session) keepAwake() {
	switch {
	case len(s.jobs) > 0 && s.awake == nil:
		// The hold is the session's: under a handler's owner, as a page's button, it would end with that region.
		s.cx.Run(func() {
			s.awake = s.cx.KeepAwake(locales.SessionDownloading(s.cx), core.KeepAwakeOptions{Display: true})
		})
	case len(s.jobs) == 0 && s.awake != nil:
		s.awake()
		s.awake = nil
	}
}

// partSize is how much of f's video its part file holds, 0 for none yet.
func partSize(st *store.Store, f store.Film) int64 {
	info, err := os.Stat(filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), store.PartFile))
	if err != nil {
		return 0
	}
	return info.Size()
}
