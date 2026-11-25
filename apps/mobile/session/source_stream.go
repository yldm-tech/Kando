package session

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
	gosync "sync"
	"sync/atomic"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"
	"kando/packages/sync"
)

// Streaming from a source (docs/media-sources.md 10.3): a video of a NAS or a server plays at once, nothing of it downloaded, as the system's player opens it at a URL of the phone's loopback server (kando/apps/mobile/loopback), the one a film being made plays from, which serves the file's bytes read through a connection of the film's own (remotefs.Handler: Range, If-Range and the rest). The film is kept as a film kept to stream (store.Film.Streamed), so the player keeps where it was left as for any film, and a download of it later goes on from there. Its URL and its connection last while the player holds the film, and go as it plays another or the film leaves the phone (EndStream). A connection that fails, as the source is switched off or the phone leaves its network, is told (StreamProblem), and the next request the player makes connects anew.

// sourceStream is the file at path of the source id that the player streams the film kept under key from: the connection made for it, the first time the player asks, and the error of the last request that failed, nil once one succeeds again. Its fields under mu are read and written off the engine's goroutine, by the loopback server's requests.
type sourceStream struct {
	s    *Session
	key  string
	id   string
	path string

	// stale is set once the source changed or went (writeSources): the connection made before is closed at the next request.
	stale atomic.Bool

	mu     gosync.Mutex
	r      nas.Reader
	closed bool
	failed error
}

// StreamSource plays the video at p of the source id, as info lists it, at once: a film already kept under its key plays as it is, from the phone when its video is downloaded and else from the source; a new one is kept as a film kept to stream, named after the file in a library named after the source as a download of it is (DownloadSource), and then play runs with its key.
func (s *Session) StreamSource(id, p string, info remotefs.Info, play func(key string)) {
	key := SourceFilmKey(id, p)
	if _, ok := s.Film(key); ok {
		play(key)
		return
	}
	rec, ok := s.sourceFilm(id, p, info)
	if !ok {
		return
	}
	rec.Streamed = true
	s.films.Set(append([]store.Film{rec}, s.films.Get()...))
	s.saveFilm(rec)
	s.matchLines()
	play(key)
}

// sourceFilm is the record of the film of the file at p of the source id, as info lists it, not kept yet; false when the source was removed.
func (s *Session) sourceFilm(id, p string, info remotefs.Info) (store.Film, bool) {
	src, ok := s.Source(id)
	if !ok {
		return store.Film{}, false
	}
	name := path.Base(p)
	title := strings.TrimSuffix(name, path.Ext(name))
	return store.Film{Desktop: store.SourceOwner(id), Library: src.Name, Source: id, Path: p, Added: s.cfg.Now(),
		Film: sync.Film{ID: store.SourceFilmID(p), Title: title, Source: sourceFilmSource, VideoSize: info.Size, Updated: info.Modified}}, true
}

// sourceLink is the URL the player streams f, a film of a source, from: the loopback server's, whose first request connects to the source; "" when the server cannot start. It connects to nothing itself, as it is read in a binding.
func (s *Session) sourceLink(f store.Film) string {
	srv := s.loopback()
	if srv == nil {
		return ""
	}
	key := f.Key()
	s.src.mu.Lock()
	st := s.src.streams[key]
	if st == nil {
		st = &sourceStream{s: s, key: key, id: f.Source, path: f.Path}
		s.src.streams[key] = st
	}
	s.src.mu.Unlock()
	return srv.URL(streamKey(key), path.Base(f.Path), st)
}

// streamKey is what the loopback server holds the stream of the film kept under key by, apart from a film being made's.
func streamKey(key string) string { return "source/" + key }

// EndStream stops streaming the film kept under key from its source, as the player lets it go: its URL is revoked, a request with it being 404 from then on, its connection closes, and what failed of it is forgotten. A film that does not stream from a source has nothing to end. Engine goroutine only.
func (s *Session) EndStream(key string) {
	s.src.mu.Lock()
	st := s.src.streams[key]
	delete(s.src.streams, key)
	s.src.mu.Unlock()
	if st == nil {
		return
	}
	if srv := s.making.server; srv != nil {
		srv.Revoke(streamKey(key))
	}
	if _, ok := s.src.streamFail.GetUntracked()[key]; ok {
		m := maps.Clone(s.src.streamFail.GetUntracked())
		delete(m, key)
		s.src.streamFail.Set(m)
	}
	s.work.Go(st.close)
}

// StreamProblem is why the film kept under key could not be read from its source the last time the player asked, nil while it streams or for a film that does not stream from a source: ErrSourceGone for a source removed, ErrStreamFileGone for a file no longer there, else the connection's error, which remotefs.Diagnose tells. Read it in a binding.
func (s *Session) StreamProblem(key string) error { return s.src.streamFail.Get()[key] }

// ErrStreamFileGone is the error of a film streamed from a source whose file is no longer there, as it was moved or deleted.
var ErrStreamFileGone = streamFileGoneError{}

type streamFileGoneError struct{}

func (streamFileGoneError) Error() string { return "the file is no longer on the source" }

// ServeHTTP answers a request of the player for the film's file: 502 when the source cannot be reached, as remotefs.Handler answers when it fails meanwhile.
func (st *sourceStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reader, err := st.open()
	if err != nil {
		st.fail(r.Context(), reader, err)
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return
	}
	remotefs.Handler(streamReader{st, reader}, st.path).ServeHTTP(w, r)
}

// open is the stream's connection, made with the source as sources.json keeps it now, and its secrets, the first time, or again after one failed.
func (st *sourceStream) open() (nas.Reader, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return nil, errStreamEnded
	}
	if st.stale.Swap(false) && st.r != nil {
		_ = st.r.Close()
		st.r = nil
	}
	if st.r != nil {
		return st.r, nil
	}
	s := st.s
	s.src.mu.Lock()
	i := slices.IndexFunc(s.src.saved, func(x store.Source) bool { return x.ID == st.id })
	var src store.Source
	if i >= 0 {
		src = s.src.saved[i]
	}
	d := s.src.dial
	s.src.mu.Unlock()
	if i < 0 {
		return nil, ErrSourceGone
	}
	// The connection outlives the request that made it, which the player may cancel as it seeks.
	ctx, cancel := context.WithTimeout(s.ctx, sourceTimeout)
	defer cancel()
	r, err := d.Open(ctx, nas.Remote(src, s.secretsOf(src, nil)))
	if err != nil {
		return nil, err
	}
	st.r = r
	return r, nil
}

// fail notes err, of a request with ctx through reader, nil before any connection: the connection is closed, so the next request connects anew, and the error is told (StreamProblem). A request the player itself gave up, as it seeks, failed of nothing.
func (st *sourceStream) fail(ctx context.Context, reader nas.Reader, err error) {
	if ctx.Err() != nil || errors.Is(err, errStreamEnded) {
		return
	}
	if errors.Is(err, remotefs.ErrNotExist) {
		err = ErrStreamFileGone
	}
	st.mu.Lock()
	if reader != nil && st.r == reader {
		st.r = nil
		_ = reader.Close()
	}
	changed := st.failed == nil || st.failed.Error() != err.Error()
	st.failed = err
	st.mu.Unlock()
	if changed {
		st.tell(err)
	}
}

// succeeded notes a request that read the file, so what failed before is forgotten.
func (st *sourceStream) succeeded() {
	st.mu.Lock()
	was := st.failed
	st.failed = nil
	st.mu.Unlock()
	if was != nil {
		st.tell(nil)
	}
}

// tell posts err as the film's StreamProblem, nil for none, unless the stream ended meanwhile.
func (st *sourceStream) tell(err error) {
	s := st.s
	select {
	case <-s.closed:
		return
	default:
	}
	s.cx.Post(func() {
		s.src.mu.Lock()
		current := s.src.streams[st.key] == st
		s.src.mu.Unlock()
		if !current {
			return
		}
		m := maps.Clone(s.src.streamFail.GetUntracked())
		if err == nil {
			delete(m, st.key)
		} else {
			m[st.key] = err
		}
		s.src.streamFail.Set(m)
	})
}

// close closes the stream's connection, and keeps it from making another.
func (st *sourceStream) close() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.closed = true
	if st.r != nil {
		_ = st.r.Close()
		st.r = nil
	}
}

// errStreamEnded is the error of a request that came after the stream ended.
var errStreamEnded = errors.New("the stream ended")

// streamReader is the stream's connection as remotefs.Handler reads it: what fails is noted (sourceStream.fail), and what reads is too (succeeded).
type streamReader struct {
	st *sourceStream
	r  nas.Reader
}

func (sr streamReader) Stat(ctx context.Context, p string) (remotefs.Info, error) {
	info, err := sr.r.Stat(ctx, p)
	if err != nil {
		sr.st.fail(ctx, sr.r, err)
	}
	return info, err
}

func (sr streamReader) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	body, err := sr.r.Open(ctx, p, offset, length)
	if err != nil {
		sr.st.fail(ctx, sr.r, err)
		return nil, err
	}
	sr.st.succeeded()
	return streamBody{body, ctx, sr}, nil
}

// streamBody is the file read from the stream's connection: a read that fails, as the connection drops midway, is noted.
type streamBody struct {
	io.ReadCloser
	ctx context.Context
	sr  streamReader
}

func (b streamBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && err != io.EOF {
		b.sr.st.fail(b.ctx, b.sr.r, err)
	}
	return n, err
}
