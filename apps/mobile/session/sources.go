package session

import (
	"context"
	"errors"
	"maps"
	"net/netip"
	"slices"
	"strings"
	gosync "sync"
	"time"

	"kando/apps/mobile/download"
	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"

	"github.com/nsgo-dev/nsgo/core"
)

// sources are the NAS and servers the phone reaches itself, without the computer (docs/media-sources.md 10): the list as the screens read it, why each film streamed from one could not be read (streamFail, by key, source_stream.go), and, under mu, the list sources.json holds (saved), the connections open (readers, one per source, made when first needed and closed as a source changes, goes or the session ends), the films the player streams from one, by key, each with a connection of its own (streams), and the Dialer they are made with. mu also orders the writes of sources.json and the secrets, which run on goroutines of their own.
type sources struct {
	list       *core.Signal[[]store.Source]
	streamFail *core.Signal[map[string]error]
	// fromComputer are the films of a source matched to a computer's film that could not be read from their source this session, by key, which stream from the computer instead (matched.go, StreamLink); probing the films whose source DownloadCopy is asking whether it answers. Engine goroutine only.
	fromComputer *core.Signal[map[string]bool]
	probing      map[string]bool

	mu      gosync.Mutex
	saved   []store.Source
	readers map[string]nas.Reader
	streams map[string]*sourceStream
	dial    nas.Dialer
}

func newSources(cx *core.Scope) *sources {
	return &sources{list: core.NewSignalFunc[[]store.Source](cx, nil, nil), streamFail: core.NewSignalFunc(cx, map[string]error{}, nil),
		fromComputer: core.NewSignalFunc(cx, map[string]bool{}, nil), probing: map[string]bool{},
		readers: map[string]nas.Reader{}, streams: map[string]*sourceStream{}, dial: nas.Remotefs}
}

// sourceTimeout bounds a Test and a folder's listing: connecting, signing in and reading the folder.
const sourceTimeout = 30 * time.Second

// SetDialer makes the session reach its sources with d in place of remotefs's servers, as a test's fakes do; before any source is read.
func (s *Session) SetDialer(d nas.Dialer) {
	s.src.mu.Lock()
	s.src.dial = d
	s.src.mu.Unlock()
}

// loadSources takes the sources read from sources.json, as the session starts.
func (s *Session) loadSources(list []store.Source) {
	s.src.mu.Lock()
	s.src.saved = slices.Clone(list)
	s.src.mu.Unlock()
	s.src.list.Set(list)
}

// Sources are the NAS and servers added, in the order added.
func (s *Session) Sources() []store.Source { return s.src.list.Get() }

// Source is the source id.
func (s *Session) Source(id string) (store.Source, bool) {
	list := s.src.list.Get()
	i := slices.IndexFunc(list, func(x store.Source) bool { return x.ID == id })
	if i < 0 {
		return store.Source{}, false
	}
	return list[i], true
}

// SaveSource keeps src, a new source when its ID is "", in place of the source of its ID or after the others, with the secrets typed, a secret not typed keeping the one kept; done runs on the engine's goroutine with the source as kept, or with why it was not. The secrets go to the secure store first, then the source to sources.json, off the engine's goroutine, as the system may make them wait; a source without a name is named after its server. Its connection, made with what it was, closes.
func (s *Session) SaveSource(src store.Source, typed map[string]string, done func(store.Source, error)) {
	if src.ID == "" {
		src.ID = nas.NewID()
	}
	src.Name = strings.TrimSpace(src.Name)
	if src.Name == "" {
		src.Name = nas.HostOf(src)
	}
	src.Config = nas.Config(src.Kind, src.Config)
	secrets := map[string]string{}
	for _, key := range nas.SecretKeys(src.Kind) {
		if v := typed[key]; v != "" {
			secrets[key] = v
		}
	}
	_, existed := s.Source(src.ID)
	s.work.Go(func() {
		list, err := s.writeSources(func(list []store.Source) ([]store.Source, error) {
			for _, key := range slices.Sorted(maps.Keys(secrets)) {
				if err := s.secrets.Set(store.SourceSecretAccount(src.ID, key), secrets[key]); err != nil {
					return nil, err
				}
			}
			if i := slices.IndexFunc(list, func(x store.Source) bool { return x.ID == src.ID }); i >= 0 {
				list[i] = src
				return list, nil
			}
			return append(list, src), nil
		}, src.ID)
		if err != nil && !existed {
			// A new source not kept keeps no secret either.
			for key := range secrets {
				_ = s.secrets.Delete(store.SourceSecretAccount(src.ID, key))
			}
		}
		s.cx.Post(func() {
			if err == nil {
				s.src.list.Set(list)
			}
			done(src, err)
		})
	})
}

// RemoveSource forgets the source id and its secrets, closing its connection and stopping its downloads, which stay paused; the films downloaded from it stay on the phone, as a computer's do once it is unpaired, and nothing on the source is touched.
func (s *Session) RemoveSource(id string) {
	for key, j := range s.jobs {
		if f, ok := s.Film(key); ok && f.Source == id {
			j.paused, j.again = true, false
			j.cancel()
			s.setTransfer(key, func(t Transfer) Transfer { t.State = Paused; return t })
		}
	}
	src, ok := s.Source(id)
	if !ok {
		return
	}
	s.src.list.Set(slices.DeleteFunc(slices.Clone(s.src.list.Get()), func(x store.Source) bool { return x.ID == id }))
	s.work.Go(func() {
		_, err := s.writeSources(func(list []store.Source) ([]store.Source, error) {
			return slices.DeleteFunc(list, func(x store.Source) bool { return x.ID == id }), nil
		}, id)
		if err != nil {
			return
		}
		for _, key := range nas.SecretKeys(src.Kind) {
			_ = s.secrets.Delete(store.SourceSecretAccount(id, key))
		}
	})
}

// writeSources changes the list sources.json holds with change and writes it, closing the connection of the source id, which changed, and its streams' at their next request; it returns the list written. It runs off the engine's goroutine, under the sources' lock, so writes land in the order they were made.
func (s *Session) writeSources(change func([]store.Source) ([]store.Source, error), id string) ([]store.Source, error) {
	s.src.mu.Lock()
	defer s.src.mu.Unlock()
	list, err := change(slices.Clone(s.src.saved))
	if err == nil {
		err = s.st.SaveSources(list)
	}
	if err != nil {
		return nil, err
	}
	s.src.saved = list
	if r := s.src.readers[id]; r != nil {
		_ = r.Close()
		delete(s.src.readers, id)
	}
	// A film streaming from it connects anew at its next request, with what the source is now, or not at all once it is removed.
	for _, st := range s.src.streams {
		if st.id == id {
			st.stale.Store(true)
		}
	}
	return slices.Clone(list), nil
}

// secretsOf are the secrets of src: those typed, else those the secure store keeps for a source saved. It reads the store, which may make it wait, so it runs off the engine's goroutine; a secret it cannot read is left out, so the connection fails at signing in and says so.
func (s *Session) secretsOf(src store.Source, typed map[string]string) map[string]string {
	out := maps.Clone(typed)
	if out == nil {
		out = map[string]string{}
	}
	for _, key := range nas.SecretKeys(src.Kind) {
		if out[key] != "" || src.ID == "" {
			continue
		}
		if v, ok, err := s.secrets.Get(store.SourceSecretAccount(src.ID, key)); err == nil && ok {
			out[key] = v
		}
	}
	return out
}

// Tested is how a Test of a source went: Err is nil once it connected and read its root, whose folders are Shares for an SMB source without a share, which lists the server's shares there.
type Tested struct {
	Err    error
	Shares []string
}

// TestSource connects to src, as typed in its form (its ID "" for a new one), with the secrets typed, else those kept, reads its root and closes the connection again; done hears how it went on the engine's goroutine. The local network is asked for first (LocalNetwork).
func (s *Session) TestSource(src store.Source, typed map[string]string, done func(Tested)) {
	src.Config = nas.Config(src.Kind, src.Config)
	s.LocalNetwork(src, func(err error) {
		if err != nil {
			done(Tested{Err: err})
			return
		}
		s.src.mu.Lock()
		d := s.src.dial
		s.src.mu.Unlock()
		s.work.Go(func() {
			ctx, cancel := context.WithTimeout(s.ctx, sourceTimeout)
			defer cancel()
			r := nas.Remote(src, s.secretsOf(src, typed))
			var t Tested
			reader, err := d.Open(ctx, r)
			if err == nil {
				var infos []remotefs.Info
				infos, err = reader.List(ctx, "")
				if r.Kind == remotefs.KindSMB && r.Share == "" {
					for _, info := range infos {
						t.Shares = append(t.Shares, info.Name)
					}
				}
				_ = reader.Close()
			}
			t.Err = err
			s.cx.Post(func() { done(t) })
		})
	})
}

// ErrLocalNetwork is the error of a source on the local network that the user did not let the app reach (core.PermissionLocalNetwork).
var ErrLocalNetwork = localNetworkError{}

type localNetworkError struct{}

func (localNetworkError) Error() string { return "the local network is not allowed" }

// LocalNetwork asks for the local network, as iOS asks the user before the app first reaches it, so its question shows at once, before the first connection to src, and a refusal is told for what it is; then is called on the engine's goroutine with ErrLocalNetwork when the user refused and src's server is on the local network (nas.Local), else nil. Elsewhere the answer is not gated.
func (s *Session) LocalNetwork(src store.Source, then func(error)) {
	host := nas.HostOf(src)
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil && a.IsLoopback() {
		// This device itself is not the local network: nothing to ask.
		then(nil)
		return
	}
	s.cx.RequestPermission(core.PermissionLocalNetwork, func(st core.PermissionStatus) {
		if st == core.PermissionDenied && nas.Local(host) {
			then(ErrLocalNetwork)
			return
		}
		then(nil)
	})
}

// reader is the connection to src, made the first time with its secrets kept; it runs off the engine's goroutine.
func (s *Session) reader(ctx context.Context, src store.Source) (nas.Reader, error) {
	s.src.mu.Lock()
	r := s.src.readers[src.ID]
	d := s.src.dial
	s.src.mu.Unlock()
	if r != nil {
		return r, nil
	}
	r, err := d.Open(ctx, nas.Remote(src, s.secretsOf(src, nil)))
	if err != nil {
		return nil, err
	}
	s.src.mu.Lock()
	defer s.src.mu.Unlock()
	if other := s.src.readers[src.ID]; other != nil {
		// Another connection was made meanwhile: the one kept is used.
		_ = r.Close()
		return other, nil
	}
	if !slices.ContainsFunc(s.src.saved, func(x store.Source) bool {
		return x.ID == src.ID && maps.Equal(x.Config, src.Config) && x.Kind == src.Kind
	}) {
		// The source changed or went meanwhile: the connection is not one to use.
		_ = r.Close()
		return nil, errSourceChanged
	}
	s.src.readers[src.ID] = r
	return r, nil
}

// closeSources closes every connection, the streams' too, as the session ends.
func (s *Session) closeSources() {
	s.src.mu.Lock()
	defer s.src.mu.Unlock()
	for id, r := range s.src.readers {
		_ = r.Close()
		delete(s.src.readers, id)
	}
	for key, st := range s.src.streams {
		st.close()
		delete(s.src.streams, key)
	}
}

// ListSource reads the folder dir of the source id, its root for "", off the engine's goroutine, and done hears its folders and videos (nas.Listing), or why not, on the engine's goroutine, unless the call it returns cancelled it first, as a page that went away does. A listing that failed closes the connection, so the next one, as Try Again, connects anew.
func (s *Session) ListSource(id, dir string, done func([]remotefs.Info, error)) (cancel func()) {
	src, ok := s.Source(id)
	if !ok {
		done(nil, ErrSourceGone)
		return func() {}
	}
	page, stop := context.WithCancel(s.ctx)
	s.LocalNetwork(src, func(err error) {
		if page.Err() != nil {
			return
		}
		if err != nil {
			done(nil, err)
			return
		}
		s.work.Go(func() {
			ctx, cancel := context.WithTimeout(page, sourceTimeout)
			defer cancel()
			var infos []remotefs.Info
			r, err := s.reader(ctx, src)
			if err == nil {
				if infos, err = r.List(ctx, dir); err != nil {
					s.dropReader(src.ID, r)
				}
			}
			s.cx.Post(func() {
				if page.Err() != nil {
					return
				}
				done(nas.Listing(infos), err)
			})
		})
	})
	return stop
}

// dropReader closes r, the connection to the source id, and forgets it when it is the one kept.
func (s *Session) dropReader(id string, r nas.Reader) {
	s.src.mu.Lock()
	defer s.src.mu.Unlock()
	if s.src.readers[id] == r {
		delete(s.src.readers, id)
	}
	_ = r.Close()
}

// errSourceChanged is the error of a connection made with what a source was before it changed or went.
var errSourceChanged = errors.New("the source changed while the phone connected to it")

// ErrSourceGone is the error of a source that was removed.
var ErrSourceGone = sourceGoneError{}

type sourceGoneError struct{}

func (sourceGoneError) Error() string { return "the source was removed" }

// SourceFilmKey is the key (store.Key) of the film of the file at p of the source id, which DownloadSource keeps it under.
func SourceFilmKey(id, p string) string {
	return store.Key(store.SourceOwner(id), store.SourceFilmID(p))
}

// DownloadSource starts downloading the video at p of the source id, as info lists it, to the phone, or goes on with its download (start): a film named after the file, kept under the source (store.SourceOwner) in a library named after it, which plays offline with the player as a computer's film does, without subtitles.
func (s *Session) DownloadSource(id, p string, info remotefs.Info) {
	key := SourceFilmKey(id, p)
	if kept, ok := s.Film(key); ok {
		s.start(kept)
		return
	}
	rec, ok := s.sourceFilm(id, p, info)
	if !ok {
		return
	}
	s.films.Set(append([]store.Film{rec}, s.films.Get()...))
	s.start(rec)
	s.matchLines()
}

// sourceFilmSource is what a film downloaded from a source is filed under among the Library's chips: a file of the user's own, as one the computer imported from its disk (catalog.SourceLocal).
const sourceFilmSource = "local"

// sourceDownload is where the video of f, a film downloaded from a source, comes from: its file, read through the source's connection, which is made off the engine's goroutine once the download asks for it; false when the source was removed.
func (s *Session) sourceDownload(f store.Film) (download.Source, bool) {
	src, ok := s.Source(f.Source)
	if !ok {
		return nil, false
	}
	return download.FromSource{Reader: func(ctx context.Context) (nas.Reader, error) { return s.reader(ctx, src) }, Path: f.Path}, true
}
