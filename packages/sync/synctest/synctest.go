// Package synctest is a stand-in for Kando's phone sync server on loopback, speaking protocols v1 and v2 (package sync) from memory, for the phone's tests: it pairs with the code it offers, lists its libraries and films, and serves their subtitles, posters and videos by ranges with an ETag. A video can be made to stall partway (Stall), as a network that drops does, so a download's pause and resume can be tested. Over v2 it takes the links a phone sends into a queue of its own, which the test moves along (jobs.go), and keeps each library's learning state in a ledger, which the test changes as the desktop would (state.go).
package synctest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	gosync "sync"
	"testing"
	"time"

	"kando/packages/sync"
)

// Film is a film the desktop serves: what it lists (sync.Film, whose VideoSize and Poster it fills in), the library it is in, and its files.
type Film struct {
	sync.Film
	Library string
	Video   []byte
	// ContentType is the video's type, video/mp4 when "".
	ContentType string
	// Poster is the poster's JPEG bytes, none when nil.
	Poster []byte
	Cues   []sync.Cue
}

// Desktop is the stand-in server. Its methods may be called from any goroutine.
type Desktop struct {
	// Addr is where it listens on loopback, and Fingerprint its certificate's (sync.Fingerprint).
	Addr        netip.AddrPort
	Fingerprint []byte
	// Name is the desktop's name, as a pairing answers it.
	Name string

	srv    *httptest.Server
	mu     gosync.Mutex
	code   string
	used   bool
	tries  int
	tokens map[string]bool
	// names are the paired devices' names by token, which v2's calls name the caller by.
	names map[string]string
	queue queue
	// ledgers are the libraries' learning state, by library ID (state.go).
	ledgers map[string]*sync.Ledger
	// target is the language the desktop translates into (settings.go).
	target string
	// refuse, putHeld and puts are how the learning state's puts are answered and how many were (state.go).
	refuse  func(sync.Record) bool
	putHeld chan struct{}
	puts    int
	// study is the lookups and explanations (study.go).
	study study
	desk  *sync.Desk
	libs  []sync.Library
	films []Film
	stall int64
	held  chan struct{}
	// off hangs up on every connection, as a computer asleep or with Phone Sync off (SetOff).
	off     bool
	ranges  []string
	modTime time.Time
}

// New starts a desktop named name on loopback, closed when the test ends. It offers no code until Offer.
func New(t testing.TB, name string) *Desktop {
	t.Helper()
	d := &Desktop{Name: name, tokens: map[string]bool{}, names: map[string]string{}, modTime: time.Unix(1_700_000_000, 0), target: "zh-Hans"}
	d.queue.receive = sync.ReceiveQueue
	d.desk = &sync.Desk{Service: jobService{d}, State: stateService{d}, Settings: settingsService{d}, Study: studyService{d}, Journal: sync.NewJournal(), Desktop: name}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+sync.PathPair, d.pair)
	mux.HandleFunc("GET "+sync.PathLibraries, d.authed(d.libraries))
	mux.HandleFunc("GET "+sync.PathFilms, d.authed(d.filmList))
	mux.HandleFunc("GET "+sync.PathSubtitles, d.authed(d.subtitles))
	mux.HandleFunc("GET "+sync.PathPoster, d.authed(d.poster))
	mux.HandleFunc("GET "+sync.PathVideo, d.authed(d.video))
	mux.HandleFunc("POST "+sync.PathCall, d.authed(d.call))
	d.srv = httptest.NewUnstartedServer(mux)
	d.srv.Listener = offListener{d.srv.Listener, d}
	// A phone reading the certificate alone (sync.FetchFingerprint) hangs up after the handshake, which the server would log as an error.
	d.srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	d.srv.StartTLS()
	t.Cleanup(d.srv.Close)
	u, err := url.Parse(d.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	d.Addr = netip.MustParseAddrPort(u.Host)
	d.Fingerprint = sync.Fingerprint(d.srv.Certificate().Raw)
	return d
}

// Offer offers code for pairing, as Settings › Phone Sync shows a new one: good for one pairing and sync.CodeTries wrong tries.
func (d *Desktop) Offer(code string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.code, d.used, d.tries = code, false, 0
}

// AddLibrary adds a library with no films yet.
func (d *Desktop) AddLibrary(id, name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.libs = append(d.libs, sync.Library{ID: id, Name: name})
}

// RemoveLibrary takes library id off the list, as a library locked on the desktop is: its films and its learning state stay, and AddLibrary brings it back.
func (d *Desktop) RemoveLibrary(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.libs = slices.DeleteFunc(d.libs, func(l sync.Library) bool { return l.ID == id })
}

// AddFilm adds f to its library.
func (d *Desktop) AddFilm(f Film) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f.VideoSize = int64(len(f.Video))
	if f.Poster != nil {
		f.Film.Poster = sync.PosterPath(f.ID)
	}
	if f.ContentType == "" {
		f.ContentType = "video/mp4"
	}
	d.films = append(d.films, f)
	for i := range d.libs {
		if d.libs[i].ID == f.Library {
			d.libs[i].Films++
		}
	}
}

// ReplaceVideo gives film id another video, whose ETag differs from the one before.
func (d *Desktop) ReplaceVideo(id string, video []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.films {
		if d.films[i].ID == id {
			d.films[i].Video, d.films[i].VideoSize = video, int64(len(video))
		}
	}
}

// Stall makes each video answer begun from now on send after bytes, then wait until the phone gives up on it, as a network that drops does; 0 sends them whole.
func (d *Desktop) Stall(after int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stall = after
}

// Hold makes each libraries answer begun from now on wait until release is called or the phone gives up on it, as a desktop slow to answer does.
func (d *Desktop) Hold() (release func()) {
	held := make(chan struct{})
	d.mu.Lock()
	d.held = held
	d.mu.Unlock()
	var once gosync.Once
	return func() {
		once.Do(func() {
			d.mu.Lock()
			if d.held == held {
				d.held = nil
			}
			d.mu.Unlock()
			close(held)
		})
	}
}

// Ranges are the video requests' Range headers, oldest first, each with its If-Range after a space when it had one.
func (d *Desktop) Ranges() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.ranges)
}

// Paired is how many devices hold a token.
func (d *Desktop) Paired() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tokens)
}

// SetOff drops every request from now on while off, as a computer asleep or with Phone Sync off does, and answers them again once it is back on, at the same address and with the same tokens, which Close cannot.
func (d *Desktop) SetOff(off bool) {
	d.mu.Lock()
	d.off = off
	d.mu.Unlock()
	if off {
		d.srv.CloseClientConnections()
	}
}

// offListener hangs up on the connections it accepts while its desktop is off, before the TLS handshake, so a phone's request fails as one to a computer that is not there does.
type offListener struct {
	net.Listener
	d *Desktop
}

func (l offListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		l.d.mu.Lock()
		off := l.d.off
		l.d.mu.Unlock()
		if !off {
			return c, nil
		}
		c.Close()
	}
}

// Close stops the desktop, as one turned off or with Phone Sync off: nothing listens at Addr after it.
func (d *Desktop) Close() { d.srv.Close() }

// RevokeAll revokes every device's token, as Settings › Phone Sync's Revoke does.
func (d *Desktop) RevokeAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	clear(d.tokens)
}

func (d *Desktop) pair(w http.ResponseWriter, r *http.Request) {
	var req sync.PairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, sync.CodeInvalid)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case d.code == "":
		fail(w, http.StatusForbidden, sync.CodeNoCode)
		return
	case d.used:
		fail(w, http.StatusForbidden, sync.CodeCodeUsed)
		return
	case d.tries >= sync.CodeTries:
		fail(w, http.StatusForbidden, sync.CodeCodeExpired)
		return
	case req.Code != d.code:
		d.tries++
		fail(w, http.StatusForbidden, sync.CodeWrongCode)
		return
	}
	d.used = true
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	d.tokens[token] = true
	d.names[token] = req.Device
	writeJSON(w, sync.PairResponse{Token: token, DeviceID: "dev-" + token[:6], Desktop: d.Name})
}

func (d *Desktop) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bytes.CutPrefix([]byte(r.Header.Get("Authorization")), []byte("Bearer "))
		d.mu.Lock()
		known := ok && d.tokens[string(token)]
		d.mu.Unlock()
		if !known {
			fail(w, http.StatusUnauthorized, sync.CodeUnauthorized)
			return
		}
		next(w, r)
	}
}

func (d *Desktop) libraries(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	held := d.held
	d.mu.Unlock()
	if held != nil {
		select {
		case <-held:
		case <-r.Context().Done():
			return
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	writeJSON(w, sync.Libraries{Libraries: slices.Clone(d.libs)})
}

func (d *Desktop) filmList(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !slices.ContainsFunc(d.libs, func(l sync.Library) bool { return l.ID == r.PathValue("lib") }) {
		fail(w, http.StatusNotFound, sync.CodeNotFound)
		return
	}
	out := sync.Films{Films: []sync.Film{}}
	for _, f := range d.films {
		if f.Library == r.PathValue("lib") {
			out.Films = append(out.Films, f.Film)
		}
	}
	writeJSON(w, out)
}

// film is film {id} of the request, false after answering 404.
func (d *Desktop) film(w http.ResponseWriter, r *http.Request) (Film, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, f := range d.films {
		if f.ID == r.PathValue("id") {
			return f, true
		}
	}
	fail(w, http.StatusNotFound, sync.CodeNotFound)
	return Film{}, false
}

func (d *Desktop) subtitles(w http.ResponseWriter, r *http.Request) {
	if f, ok := d.film(w, r); ok {
		writeJSON(w, sync.Subtitles{Cues: append([]sync.Cue{}, f.Cues...)})
	}
}

func (d *Desktop) poster(w http.ResponseWriter, r *http.Request) {
	f, ok := d.film(w, r)
	if !ok {
		return
	}
	if f.Poster == nil {
		fail(w, http.StatusNotFound, sync.CodeNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(f.Poster)
}

func (d *Desktop) video(w http.ResponseWriter, r *http.Request) {
	f, ok := d.film(w, r)
	if !ok {
		return
	}
	d.mu.Lock()
	rec := r.Header.Get("Range")
	if ir := r.Header.Get("If-Range"); ir != "" {
		rec += " " + ir
	}
	d.ranges = append(d.ranges, rec)
	stall, mod := d.stall, d.modTime
	d.mu.Unlock()
	sum := sha256.Sum256(f.Video)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:8])+`"`)
	w.Header().Set("Content-Type", f.ContentType)
	content := &stallReader{Reader: bytes.NewReader(f.Video), left: stall, ctx: r.Context()}
	http.ServeContent(flushWriter{w}, r, "", mod, content)
}

// stallReader reads its bytes until left of them are read, when left was set, then waits for ctx to end.
type stallReader struct {
	*bytes.Reader
	left int64
	ctx  context.Context
}

func (s *stallReader) Read(p []byte) (int, error) {
	if s.left == 0 {
		return s.Reader.Read(p)
	}
	if s.left < 0 {
		<-s.ctx.Done()
		return 0, s.ctx.Err()
	}
	if int64(len(p)) > s.left {
		p = p[:s.left]
	}
	n, err := s.Reader.Read(p)
	if s.left -= int64(n); s.left == 0 {
		s.left = -1
	}
	return n, err
}

// flushWriter sends each write at once, so the bytes a stalled answer has read reach the phone before it waits.
type flushWriter struct{ http.ResponseWriter }

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.ResponseWriter.Write(p)
	if fl, ok := f.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}

func fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(sync.ErrorBody{Code: code, Message: code})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
