// Package phonesync is Kando's phone sync server (yldm-tech/Kando#178, protocols v1 and v2 in kando/packages/sync): while Phone Sync is on, it serves the app's libraries over HTTPS to the phones paired with it, on the computer's local network addresses only, and answers protocol v2's calls, which send it links and follow and control its queue, through a psync.Desk whose Journal the app publishes its jobs in (PublishJobs). Its certificate, made once, and the paired devices, by the SHA-256 of their tokens, are kept in Kando's config folder. It reads the libraries through a Source, which the app answers on its own goroutine; nothing here touches the app's state.
package phonesync

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	psync "kando/packages/sync"
)

// Library is a library the app serves: its ID in the protocol, its name as the app shows it, and its folder.
type Library struct {
	ID, Name, Dir string
}

// Film is a film the app serves, done processing: its ID in the protocol, what the protocol says of it, and its files.
type Film struct {
	ID, Title, Source string
	// Spoken is the language spoken in it as whisper's code, Translation the language its subtitles are translated into, "" for one not translated.
	Spoken, Translation string
	Untranslated        int
	// Duration is how long it plays, 0 when the app does not know.
	Duration time.Duration
	// Video is its video file, Poster its poster's file, "" for none yet.
	Video, Poster string
	// Watched is a film watched to its end, last at WatchedAt, zero when that is not known; the films through the bucket clean up its video after a while (FilmsConfig.CleanAfter).
	Watched   bool
	WatchedAt time.Time
	// Subtitles is its bilingual subtitles' file, Transcript its original lines' (media.TranscriptOf) and Corrections the user's corrections of its lines (media.CorrectionsPath), each "" or missing for a film without; their times say when it last changed besides its video's (psync.Film.Updated), so a phone that has the film reads its lines again once the user corrects one.
	Subtitles, Transcript, Corrections string
}

// ErrNotFound is a Source's error for a library or film it does not serve, or a film without what was asked for.
var ErrNotFound = errors.New("not found")

// Source is the app's libraries as the server serves them. Its methods run on the requests' goroutines, so the app answers them on its own goroutine and reads files outside it.
type Source interface {
	// Libraries are the libraries the app knows, locked ones left out: those are never served.
	Libraries(ctx context.Context) ([]Library, error)
	// Films are lib's films a phone can play.
	Films(ctx context.Context, lib Library) ([]Film, error)
	// Cues are f's subtitles, line by line.
	Cues(ctx context.Context, f Film) ([]psync.Cue, error)
}

// Config is what the server is started with.
type Config struct {
	// Dir is Kando's config folder, where the certificate and the devices are kept.
	Dir string
	// Name is the computer's name, which the pairing URL and the pairing's answer carry.
	Name string
	// Port is the port to listen on, the one used last time so a paired phone finds the desktop where it was; when it is taken, or 0, the system picks one.
	Port   int
	Source Source
	// Jobs is the app's queue as protocol v2's calls act on it; nil answers only hello, with no operation but the reads.
	Jobs psync.Service
	// State is the libraries' learning state as protocol v2's calls read and write it; nil answers them unsupported.
	State psync.StateService
	// Settings is the settings a phone reads and changes over protocol v2; nil answers them unsupported.
	Settings psync.SettingsService
	// Study is the lookups and explanations protocol v2's calls ask for; nil answers them unsupported.
	Study psync.StudyService
	// Subscriptions is the subscriptions protocol v2's calls list and change; nil answers them unsupported.
	Subscriptions psync.SubscriptionService
	// Relay is the user's bucket the storage relay runs in (relay.go), nil while the relay is off. RelayHealth, when set, is told on a goroutine of the relay's after each of its turns what failed of it, nil for a turn that went through (psync.Relay.Health).
	Relay       Bucket
	RelayHealth func(err error)
	// Films, with the relay on, shares the films with the phones through the bucket (films.go); nil shares none.
	Films *FilmsConfig
	// Addrs are the addresses to listen on, looked at again every rescanEvery as the computer joins other networks; nil is LANAddrs. Tests give loopback.
	Addrs func() []netip.Addr
	// Changed is told, on any goroutine, that what the settings show changed: the code on offer, the devices, or the addresses listened on.
	Changed func()
	// Logf logs the requests, without their tokens; nil is log.Printf.
	Logf func(format string, args ...any)
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// rescanEvery is how often the server looks at the computer's addresses again.
const rescanEvery = 30 * time.Second

// sourceTimeout is how long a request waits for the app.
const sourceTimeout = 30 * time.Second

// Server is the phone sync server, listening until Close.
type Server struct {
	cfg         Config
	fingerprint []byte
	devices     *devices
	http        *http.Server
	stop        chan struct{}
	done        sync.WaitGroup

	mu     sync.Mutex
	closed bool
	port   int
	// listeners are the addresses listened on, in the order cfg.Addrs last gave them (order).
	listeners map[netip.Addr]net.Listener
	order     []netip.Addr
	offer     *offer
	limit     limiter
	// listed is each library's playable films as last read, by its ID, which a request for a film's files looks it up in for filmsFresh after (filmsFor).
	listed map[string]listing
	// desk answers protocol v2's calls; relay carries them through the user's bucket, nil while it is off.
	desk  *psync.Desk
	relay *psync.Relay
	// shared puts the films in the bucket, nil while it does not (films.go).
	shared *films
}

// listing is a library's playable films as read at a time.
type listing struct {
	films []served
	at    time.Time
}

// filmsFresh is how long a library's films as read stay good for finding a film's files, so a player reading a video by many ranges does not have the app list every library at each one. Whether the library is still served, not locked, is asked at every request.
const filmsFresh = 10 * time.Second

// Start reads or makes the certificate and reads the devices in cfg.Dir, and listens on cfg.Addrs at cfg.Port. A computer on no local network yet starts listening once it joins one.
func Start(cfg Config) (*Server, error) {
	if cfg.Dir == "" {
		return nil, errors.New("phone sync: no config folder")
	}
	if cfg.Addrs == nil {
		cfg.Addrs = LANAddrs
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Changed == nil {
		cfg.Changed = func() {}
	}
	cert, err := loadCert(cfg.Dir)
	if err != nil {
		return nil, err
	}
	devs, err := loadDevices(filepath.Join(cfg.Dir, DevicesFile))
	if err != nil {
		return nil, fmt.Errorf("read the paired phones: %w", err)
	}
	s := &Server{cfg: cfg, fingerprint: psync.Fingerprint(cert.Certificate[0]), devices: devs, stop: make(chan struct{}), port: cfg.Port, listeners: map[netip.Addr]net.Listener{}, listed: map[string]listing{}}
	jobs := cfg.Jobs
	if jobs == nil {
		jobs = noJobs{}
	}
	s.desk = &psync.Desk{Service: jobs, State: cfg.State, Settings: cfg.Settings, Study: cfg.Study, Subscriptions: cfg.Subscriptions, Journal: psync.NewJournal(), Desktop: cfg.Name, Now: cfg.Now}
	s.http = &http.Server{
		Handler:           s.routes(),
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13},
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          log.New(logWriter(cfg.Logf), "", 0),
	}
	if cfg.Relay != nil {
		s.startRelay(cfg.Relay)
		if cfg.Films != nil {
			s.startFilms(cfg.Relay, *cfg.Films)
		}
	}
	if err := s.rescan(); err != nil {
		s.Close()
		return nil, err
	}
	s.done.Add(1)
	go s.watch()
	return s, nil
}

// logWriter writes the HTTP server's own errors, as TLS handshakes that failed, through Logf.
type logWriter func(format string, args ...any)

func (w logWriter) Write(p []byte) (int, error) {
	w("phonesync: %s", strings.TrimSpace(string(p)))
	return len(p), nil
}

// watch looks at the computer's addresses every rescanEvery until Close.
func (s *Server) watch() {
	defer s.done.Done()
	tick := time.NewTicker(rescanEvery)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
			if err := s.rescan(); err != nil {
				s.cfg.Logf("phonesync: listen: %v", err)
			}
		}
	}
}

// rescan listens on the addresses cfg.Addrs gives that it does not listen on yet, all on one port, and stops listening on those it no longer gives. It fails only when it can listen on none of them.
func (s *Server) rescan() error {
	want := s.cfg.Addrs()
	changed := false
	defer func() {
		if changed {
			s.cfg.Changed()
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.order = want
	for addr, l := range s.listeners {
		if !slices.Contains(want, addr) {
			l.Close()
			delete(s.listeners, addr)
			changed = true
		}
	}
	var failed error
	for _, addr := range want {
		if _, ok := s.listeners[addr]; ok {
			continue
		}
		l, err := s.listen(addr)
		if err != nil {
			s.cfg.Logf("phonesync: listen on %s: %v", addr, err)
			failed = err
			continue
		}
		s.listeners[addr] = l
		changed = true
		s.done.Add(1)
		go func() {
			defer s.done.Done()
			if err := s.http.ServeTLS(l, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
				s.cfg.Logf("phonesync: serve on %s: %v", addr, err)
			}
		}()
	}
	if len(s.listeners) == 0 && failed != nil {
		return failed
	}
	return nil
}

// listen listens on addr at the server's port, or, before it has one or when that one is taken while it listens nowhere else, at one the system picks, which becomes the server's port. The caller holds mu.
func (s *Server) listen(addr netip.Addr) (net.Listener, error) {
	l, err := net.Listen("tcp", netip.AddrPortFrom(addr, uint16(s.port)).String())
	if err != nil && s.port != 0 && len(s.listeners) == 0 {
		l, err = net.Listen("tcp", netip.AddrPortFrom(addr, 0).String())
	}
	if err != nil {
		return nil, err
	}
	s.port = l.Addr().(*net.TCPAddr).Port
	return l, nil
}

// Close stops listening, ends the connections and forgets the code on offer.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.offer != nil && s.offer.timer != nil {
		s.offer.timer.Stop()
	}
	s.offer = nil
	for addr, l := range s.listeners {
		l.Close()
		delete(s.listeners, addr)
	}
	s.mu.Unlock()
	close(s.stop)
	err := s.http.Close()
	if errors.Is(err, net.ErrClosed) {
		// The listeners were closed above; http.Server still tracks those its Serve has not yet let go of, and closing them again reports it.
		err = nil
	}
	s.done.Wait()
	return err
}

// Port is the port the server listens on, 0 before it listens anywhere.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Addrs are where the server listens, in the order cfg.Addrs gave them when it last looked.
func (s *Server) Addrs() []netip.AddrPort {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []netip.AddrPort
	for _, addr := range s.order {
		if _, ok := s.listeners[addr]; ok {
			out = append(out, netip.AddrPortFrom(addr, uint16(s.port)))
		}
	}
	return out
}

// Fingerprint is the SHA-256 of the server's certificate, which phones pin.
func (s *Server) Fingerprint() []byte { return slices.Clone(s.fingerprint) }

// NewOffer offers a new pairing code in place of the one on offer, good for psync.CodeLifetime.
func (s *Server) NewOffer() Offer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Offer{}
	}
	s.dropOffer()
	o := &offer{code: newCode(), expires: s.cfg.Now().Add(psync.CodeLifetime)}
	o.timer = time.AfterFunc(psync.CodeLifetime, s.cfg.Changed)
	s.offer = o
	return Offer{Code: o.code, Expires: o.expires, State: OfferLive}
}

// ClearOffer takes the code on offer back, as when Phone Sync's settings close.
func (s *Server) ClearOffer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropOffer()
}

// dropOffer forgets the code on offer; the caller holds mu.
func (s *Server) dropOffer() {
	if s.offer != nil && s.offer.timer != nil {
		s.offer.timer.Stop()
	}
	s.offer = nil
}

// Offer is the code on offer and where it stands.
func (s *Server) Offer() Offer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.offer == nil {
		return Offer{}
	}
	return Offer{Code: s.offer.code, Expires: s.offer.expires, State: s.offer.state(s.cfg.Now())}
}

// Devices are the paired phones, the latest paired first.
func (s *Server) Devices() []Device { return s.devices.all() }

// Revoke unpairs device id: its token is refused from its next request on.
func (s *Server) Revoke(id string) error {
	if err := s.devices.revoke(id); err != nil {
		return err
	}
	s.desk.Forget(id)
	s.forgetRelay(id)
	s.cfg.Changed()
	return nil
}

// SetReadOnly makes device id read-only (on), so its next calls that change something are refused, or lets it change things again, and keeps that with the device.
func (s *Server) SetReadOnly(id string, on bool) error {
	if err := s.devices.setReadOnly(id, on); err != nil {
		return err
	}
	s.cfg.Changed()
	return nil
}

// routes are protocol v1's paths and v2's call.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+psync.PathPair, s.pair)
	mux.HandleFunc("GET "+psync.PathLibraries, s.authed(s.libraries))
	mux.HandleFunc("GET "+psync.PathFilms, s.authed(s.films))
	mux.HandleFunc("GET "+psync.PathSubtitles, s.authed(s.subtitles))
	mux.HandleFunc("GET "+psync.PathPoster, s.authed(s.poster))
	mux.HandleFunc("GET "+psync.PathVideo, s.authed(s.video))
	mux.HandleFunc("POST "+psync.PathCall, s.authed(s.call))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, http.StatusNotFound, psync.CodeNotFound, "no such path")
	})
	return mux
}

// logged is a response that remembers its status, for the request's line in the log.
type logged struct {
	http.ResponseWriter
	status int
}

func (l *logged) WriteHeader(status int) {
	if l.status == 0 {
		l.status = status
	}
	l.ResponseWriter.WriteHeader(status)
}

func (l *logged) Write(p []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	return l.ResponseWriter.Write(p)
}

// authed runs handle for a request whose bearer token is a paired device's, and refuses any other. Each request is logged with the device's name, never its token.
func (s *Server) authed(handle func(http.ResponseWriter, *http.Request, Device)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lw := &logged{ResponseWriter: w}
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		dev, ok, saved := s.devices.check(strings.TrimSpace(token), s.cfg.Now())
		who := "unknown device"
		if ok {
			who = strconv.Quote(dev.Name)
			if saved {
				s.cfg.Changed()
			}
			handle(lw, r, dev)
		} else {
			s.fail(lw, r, http.StatusUnauthorized, psync.CodeUnauthorized, "pair this phone with Kando first")
		}
		s.cfg.Logf("phonesync: %s %s %d, %s from %s", r.Method, r.URL.Path, lw.status, who, remoteIP(r))
	}
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// fail answers an error as the protocol does.
func (s *Server) fail(w http.ResponseWriter, _ *http.Request, status int, code, message string) {
	writeJSON(w, status, psync.ErrorBody{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// pair pairs a phone with the code on offer, within the pairing limit, and answers its device token.
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	lw := &logged{ResponseWriter: w}
	var req psync.PairRequest
	err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req)
	code, changed := s.tryPair(req.Code, err == nil)
	if changed && code != "" {
		// The last try the code had went wrong: the settings say it expired.
		s.cfg.Changed()
	}
	switch code {
	case "":
		dev, token, err := s.devices.add(req.Device, req.Platform, s.cfg.Now())
		// The code is used, and the device listed unless it could not be saved: the settings say so.
		s.cfg.Changed()
		if err != nil {
			s.fail(lw, r, http.StatusInternalServerError, psync.CodeFailed, "could not save the device")
			s.cfg.Logf("phonesync: pair: save the device: %v", err)
			break
		}
		writeJSON(lw, http.StatusOK, psync.PairResponse{Token: token, DeviceID: dev.ID, Desktop: s.cfg.Name})
		s.cfg.Logf("phonesync: paired %q (%s) from %s", dev.Name, dev.Platform, remoteIP(r))
		return
	case psync.CodeTooMany:
		s.fail(lw, r, http.StatusTooManyRequests, code, "too many pairings; wait a minute")
	case psync.CodeInvalid:
		s.fail(lw, r, http.StatusBadRequest, code, "the request is not a pairing")
	default:
		s.fail(lw, r, http.StatusForbidden, code, "the code does not pair")
	}
	s.cfg.Logf("phonesync: %s %s %d from %s", r.Method, r.URL.Path, lw.status, remoteIP(r))
}

// tryPair counts a pairing against the limit and checks its code: "" when it pairs, else the protocol's error code. A request that could not be read counts too. changed reports whether the code on offer is used or ended by it.
func (s *Server) tryPair(code string, read bool) (result string, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.cfg.Now()
	if !s.limit.allow(now) {
		return psync.CodeTooMany, false
	}
	if !read {
		return psync.CodeInvalid, false
	}
	o := s.offer
	if o == nil {
		return psync.CodeNoCode, false
	}
	before := o.state(now)
	result = o.try(strings.TrimSpace(code), now)
	return result, o.state(now) != before
}

// sourceContext is the context a request asks the app under.
func sourceContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), sourceTimeout)
}

// libraries answers the libraries, each with how many films a phone can play.
func (s *Server) libraries(w http.ResponseWriter, r *http.Request, _ Device) {
	ctx, cancel := sourceContext(r)
	defer cancel()
	libs, err := s.cfg.Source.Libraries(ctx)
	if err != nil {
		s.sourceFailed(w, r, err)
		return
	}
	out := psync.Libraries{Libraries: []psync.Library{}}
	for _, lib := range libs {
		films, err := s.filmsFor(ctx, lib, false)
		if err != nil && !errors.Is(err, ErrNotFound) {
			s.sourceFailed(w, r, err)
			return
		}
		out.Libraries = append(out.Libraries, psync.Library{ID: lib.ID, Name: lib.Name, Films: len(films)})
	}
	writeJSON(w, http.StatusOK, out)
}

// films answers the films of library {lib}.
func (s *Server) films(w http.ResponseWriter, r *http.Request, _ Device) {
	ctx, cancel := sourceContext(r)
	defer cancel()
	lib, err := s.library(ctx, r.PathValue("lib"))
	if err != nil {
		s.sourceFailed(w, r, err)
		return
	}
	films, err := s.filmsFor(ctx, lib, false)
	if err != nil {
		s.sourceFailed(w, r, err)
		return
	}
	out := psync.Films{Films: []psync.Film{}}
	for _, f := range films {
		out.Films = append(out.Films, f.film)
	}
	writeJSON(w, http.StatusOK, out)
}

// subtitles answers the lines of film {id}.
func (s *Server) subtitles(w http.ResponseWriter, r *http.Request, _ Device) {
	ctx, cancel := sourceContext(r)
	defer cancel()
	f, err := s.film(ctx, r.PathValue("id"))
	if err != nil {
		s.sourceFailed(w, r, err)
		return
	}
	cues, err := s.cfg.Source.Cues(ctx, f)
	if err != nil {
		s.sourceFailed(w, r, err)
		return
	}
	if cues == nil {
		cues = []psync.Cue{}
	}
	writeJSON(w, http.StatusOK, psync.Subtitles{Cues: cues})
}

// poster answers film {id}'s poster file.
func (s *Server) poster(w http.ResponseWriter, r *http.Request, _ Device) {
	ctx, cancel := sourceContext(r)
	f, err := s.film(ctx, r.PathValue("id"))
	cancel()
	if err == nil && f.Poster == "" {
		err = ErrNotFound
	}
	if err != nil {
		s.sourceFailed(w, r, err)
		return
	}
	s.serveFile(w, r, f.Poster)
}

// video answers film {id}'s video file, by ranges when asked, with an ETag that changes when the file does.
func (s *Server) video(w http.ResponseWriter, r *http.Request, _ Device) {
	ctx, cancel := sourceContext(r)
	f, err := s.film(ctx, r.PathValue("id"))
	cancel()
	if err != nil {
		s.sourceFailed(w, r, err)
		return
	}
	s.serveFile(w, r, f.Video)
}

// serveFile serves the file at path with http.ServeContent, which answers Range, If-Range and If-None-Match by the ETag set here: the file's size and modification time.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, path string) {
	file, err := os.Open(path)
	if err != nil {
		s.sourceFailed(w, r, fileErr(err))
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		s.sourceFailed(w, r, ErrNotFound)
		return
	}
	w.Header().Set("ETag", fileETag(info))
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func fileErr(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

// fileETag names a file as it is: its size and modification time, which change when it is written again.
func fileETag(info os.FileInfo) string {
	return `"` + strconv.FormatInt(info.Size(), 36) + "-" + strconv.FormatInt(info.ModTime().UnixNano(), 36) + `"`
}

// sourceFailed answers an error of the app's: not found, the request's own end, or anything else.
func (s *Server) sourceFailed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		s.fail(w, r, http.StatusNotFound, psync.CodeNotFound, "not found")
	case errors.Is(err, context.Canceled) && r.Context().Err() != nil:
		// The phone went away; nobody reads the answer.
	default:
		s.fail(w, r, http.StatusInternalServerError, psync.CodeFailed, "the desktop could not answer")
		s.cfg.Logf("phonesync: %s %s: %v", r.Method, r.URL.Path, err)
	}
}

// library is the library with ID id among those the app serves.
func (s *Server) library(ctx context.Context, id string) (Library, error) {
	libs, err := s.cfg.Source.Libraries(ctx)
	if err != nil {
		return Library{}, err
	}
	for _, lib := range libs {
		if lib.ID == id {
			return lib, nil
		}
	}
	return Library{}, ErrNotFound
}

// film is the playable film with ID id in any of the libraries the app serves.
func (s *Server) film(ctx context.Context, id string) (Film, error) {
	libs, err := s.cfg.Source.Libraries(ctx)
	if err != nil {
		return Film{}, err
	}
	for _, lib := range libs {
		films, err := s.filmsFor(ctx, lib, true)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return Film{}, err
		}
		for _, f := range films {
			if f.ID == id {
				return f.Film, nil
			}
		}
	}
	return Film{}, ErrNotFound
}

// filmsFor is lib's playable films: read from the app, or with cached, as last read when that was less than filmsFresh ago. lib is one the app serves now, as the caller just asked (Source.Libraries).
func (s *Server) filmsFor(ctx context.Context, lib Library, cached bool) ([]served, error) {
	now := s.cfg.Now()
	if cached {
		s.mu.Lock()
		l, ok := s.listed[lib.ID]
		s.mu.Unlock()
		if ok && now.Sub(l.at) < filmsFresh {
			return l.films, nil
		}
	}
	films, err := s.cfg.Source.Films(ctx, lib)
	if err != nil {
		s.mu.Lock()
		delete(s.listed, lib.ID)
		s.mu.Unlock()
		return nil, err
	}
	out := playable(films)
	s.mu.Lock()
	s.listed[lib.ID] = listing{films: out, at: now}
	s.mu.Unlock()
	return out, nil
}

// served is a film with what the protocol says of it.
type served struct {
	Film
	film psync.Film
}

// playable is the films of films whose video is on disk, with what their files tell: the video's size, and the latest time the video, a subtitle file or the corrections file changed.
func playable(films []Film) []served {
	var out []served
	for _, f := range films {
		info, err := os.Stat(f.Video)
		if f.Video == "" || err != nil || info.IsDir() {
			continue
		}
		updated := info.ModTime()
		for _, sub := range []string{f.Subtitles, f.Transcript, f.Corrections} {
			if si, err := os.Stat(sub); sub != "" && err == nil && si.ModTime().After(updated) {
				updated = si.ModTime()
			}
		}
		p := psync.Film{ID: f.ID, Title: f.Title, Source: f.Source, DurationMS: f.Duration.Milliseconds(), Spoken: f.Spoken, Translation: f.Translation,
			Untranslated: f.Untranslated, VideoSize: info.Size(), Updated: updated.UTC()}
		if f.Poster != "" {
			if _, err := os.Stat(f.Poster); err == nil {
				p.Poster = psync.PosterPath(f.ID)
			}
		}
		out = append(out, served{Film: f, film: p})
	}
	return out
}

// call answers a protocol v2 call from dev, as the device is now (a read-only one changes nothing, psync.Caller.ReadOnly), within callTimeout of its operation.
func (s *Server) call(w http.ResponseWriter, r *http.Request, dev Device) {
	// The body is read here to learn the operation, and handed on to HandleCall as it was; one too long or not a call is HandleCall's to refuse.
	data, err := io.ReadAll(io.LimitReader(r.Body, psync.MaxCall+1))
	if err != nil {
		data = nil
	}
	var head struct {
		Op string `json:"op"`
	}
	_ = json.Unmarshal(data, &head)
	r.Body = io.NopCloser(bytes.NewReader(data))
	ctx, cancel := context.WithTimeout(r.Context(), callTimeout(head.Op))
	defer cancel()
	psync.HandleCall(w, r.WithContext(ctx), s.desk, psync.Caller{DeviceID: dev.ID, Name: dev.Name, ReadOnly: dev.ReadOnly})
}

// callTimeout is how long a protocol v2 call of op waits for the app: sourceTimeout, which is longer than an events call waits (psync.MaxEventWait), but for subscribing, which reads the whole list with yt-dlp first and which a phone waits psync.SubscribeWait for.
func callTimeout(op string) time.Duration {
	if op == psync.OpSubscribe {
		return psync.SubscribeWait
	}
	return sourceTimeout
}

// PublishJobs makes jobs what phones are told of the app's queue, in the order a list gives them, and wakes the phones waiting for its changes. It reports whether anything changed.
func (s *Server) PublishJobs(jobs []psync.Job) bool { return s.desk.Journal.PublishJobs(jobs) }

// PublishState tells the phones waiting for events that the learning state of the library of ID lib changed (psync.EventState).
func (s *Server) PublishState(lib string) { s.desk.Journal.PublishState(lib) }

// noJobs is the queue of an app that gives the server none: it says hello, and lists no jobs.
type noJobs struct{}

func (noJobs) Hello(context.Context, psync.Caller) (psync.Hello, error) {
	return psync.Hello{Ops: []string{psync.OpHello, psync.OpJobs, psync.OpJob, psync.OpEvents}, Receive: psync.ReceiveOff}, nil
}

func (noJobs) CreateJob(context.Context, psync.Caller, psync.JobCreate) (psync.JobCreated, error) {
	return psync.JobCreated{}, psync.Errorf(psync.CodeLinksOff, "this desktop takes no links")
}

func (noJobs) ActOnJob(_ context.Context, _ psync.Caller, op, id string) (psync.Job, error) {
	return psync.Job{}, psync.Errorf(psync.CodeNotFound, "no job %q", id)
}

func (noJobs) UpdateYTDLP(context.Context, psync.Caller) (psync.ToolUpdate, error) {
	return psync.ToolUpdate{}, psync.Errorf(psync.CodeUnsupported, "this desktop updates nothing")
}
