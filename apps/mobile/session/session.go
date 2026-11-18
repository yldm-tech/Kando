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
	jobs      map[string]*job
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
	// prefsMu orders the saves of the preferences as saveMu does the desktops': prefsSaving is the generation SetPrefs last gave out, prefsSaved the one last written.
	prefsMu                 gosync.Mutex
	prefsSaving, prefsSaved uint64
}

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
		loaded:    core.NewSignal(cx, false),
		desktops:  core.NewSignalFunc[[]store.Desktop](cx, nil, nil),
		films:     core.NewSignalFunc[[]store.Film](cx, nil, nil),
		transfers: core.NewSignalFunc(cx, map[string]Transfer{}, nil),
		used:      core.NewSignal(cx, int64(0)),
		prefs:     core.NewSignalFunc(cx, store.Prefs{}, nil),
		jobs:      map[string]*job{},
		following: map[string]*follow{},
		clients:   map[string]*sync.Client{},
		posters:   map[string][]byte{},
	}
	cx.Cleanup(func() {
		for _, j := range s.jobs {
			j.cancel()
		}
		for _, fw := range s.following {
			fw.cancel()
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
		// A phone that paired or downloaded before the introduction existed has been introduced already.
		migrated := !prefs.Onboarded && (len(desktops) > 0 || len(films) > 0)
		prefs.Onboarded = prefs.Onboarded || migrated
		parts := map[string]int64{}
		for _, f := range films {
			if !f.Done() {
				parts[f.Key()] = partSize(s.st, f)
			}
		}
		cx.Post(func() {
			s.desktops.Set(desktops)
			s.films.Set(films)
			s.used.Set(used)
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
			s.loaded.Set(true)
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
	list := slices.Clone(s.desktops.Get())
	if i := slices.IndexFunc(list, func(x store.Desktop) bool { return x.ID == d.ID }); i >= 0 {
		list[i] = d
	} else {
		list = append(list, d)
	}
	s.setDesktops(list)
}

// Unpair forgets desktop id and stops its downloads; the films kept from it stay.
func (s *Session) Unpair(id string) {
	for key, j := range s.jobs {
		if f, ok := s.Film(key); ok && f.Desktop == id {
			j.paused, j.again = true, false
			j.cancel()
			s.setTransfer(key, func(t Transfer) Transfer { t.State = Paused; return t })
		}
	}
	if c, ok := s.clients[id]; ok {
		c.Close()
		delete(s.clients, id)
	}
	s.setDesktops(slices.DeleteFunc(slices.Clone(s.desktops.Get()), func(d store.Desktop) bool { return d.ID == id }))
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
				if err := s.st.SaveCues(cur.Desktop, cur.Film.ID, cues); err != nil {
					s.cx.Post(func() { delete(s.following, key) })
					return
				}
				_ = s.st.SaveFilm(cur)
				s.cx.Post(func() {
					delete(s.following, key)
					// Taken again as it stands now, so a position kept while the files were written stays.
					if cur, ok := s.followed(key, listed); ok {
						s.replace(cur)
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

// SavePosition keeps where the film under key was left playing.
func (s *Session) SavePosition(key string, at time.Duration) {
	f, ok := s.Film(key)
	if !ok || f.PositionMS == at.Milliseconds() {
		return
	}
	f.PositionMS = at.Milliseconds()
	s.replace(f)
	s.work.Go(func() { _ = s.st.SaveFilm(f) })
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
	data, _, err := c.Poster(ctx, id)
	if err != nil {
		return nil, err
	}
	s.postersMu.Lock()
	s.posters[key] = data
	s.postersMu.Unlock()
	return data, nil
}

func (s *Session) start(f store.Film) {
	key := f.Key()
	if j, running := s.jobs[key]; running || f.Done() {
		if running && j.paused && !j.remove.Load() {
			// Paused, but still finishing: the download starts again once the goroutine is done with the part file.
			j.again = true
			s.setTransfer(key, func(t Transfer) Transfer { t.State, t.Problem = Running, NoProblem; return t })
		}
		return
	}
	c := s.Client(f.Desktop)
	if c == nil {
		s.setTransfer(key, func(t Transfer) Transfer { t.State, t.Problem = Failed, Unpaired; return t })
		return
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
		got, err := download.Film(ctx, c, s.st, f, func(d, n int64) {
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
	s.work.Go(func() {
		_ = s.st.DeleteFilm(f.Desktop, f.Film.ID)
		used, _ := s.st.Used()
		s.cx.Post(func() { s.used.Set(used) })
	})
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
