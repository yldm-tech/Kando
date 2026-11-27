package main

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash/fnv"
	"log"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"kando/apps/kando/category"
	"kando/apps/kando/films"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/storage"
	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// Settings › 手机同步 (phonesync/, protocol v1 in packages/sync): while Phone Sync is on, the desktop serves its libraries to the phones paired with it on the local network. The server runs on goroutines of its own and asks the app for the libraries through phoneSource, whose answers run on the engine's goroutine (remote.OnUI); what the page shows of it, the code on offer and the devices, is copied here through update whenever the server says it changed (phoneSyncPull). A pairing code is offered only while the page shows (phoneSyncFollow), and taken back when it closes.

// phoneUI is the state of Phone Sync.
type phoneUI struct {
	// on is the switch, which settings.json keeps; port is the port the server listened on last, kept too, so a paired phone finds the desktop where it was.
	on   bool
	port int
	// server is the running server, nil while it is off or starting; running says the page shows one, with its certificate's fingerprint and the computer's name, which a screenshot sets without a server. starting is a start under way off the UI thread, and gen counts the starts and stops, so only the newest start lands. err is why the last start failed.
	server      *phonesync.Server
	running     bool
	fingerprint []byte
	name        string
	starting    bool
	gen         int
	err         string
	// shown is whether the page showed with the server running at the last afterChange, so a code is offered as it opens and taken back as it closes.
	shown bool
	// offer is the code on offer as the server last said, and qr its pairing URL's QR code, made once for each code and set of addresses (qrText).
	offer     phonesync.Offer
	qr        []byte
	qrModules int
	qrText    string
	addrs     []netip.AddrPort
	devices   []phonesync.Device
	revokeErr string
	// listen gives the addresses the server listens on, nil for the computer's local network ones (phonesync.LANAddrs); tests give loopback, and a clock of their own as now, nil for time.Now.
	listen func() []netip.Addr
	now    func() time.Time
	// receiveOff is 接收手机发来的链接 turned off, and receiveAsk 收到后 set to 先问我, which settings.json keeps (state_phonesync_jobs.go).
	receiveOff, receiveAsk bool
	// relay is the storage relay turned on, which settings.json keeps (state_phonesync_relay.go). relaySig names the store the running server's relay was started in, "" for none (phoneRelaySig), and relayArmed is a restart waiting for the store's settings to settle; relayOK is a turn of the relay that went through since it started, relayErr why its last one failed.
	relay      bool
	relaySig   string
	relayArmed bool
	relayOK    bool
	relayErr   error
	// ledgers are the libraries' learning state as phones were last told it, by library folder, each read the first time a phone asks or the open library changes (state_phonesync_state.go), and priming the folders being read so (phoneStatePrime); stateSign sums up the open library's state as phones were last told it changed (phoneSyncPublishState).
	ledgers   map[string]*phoneLedger
	priming   map[string]bool
	stateSign uint64
	// watchedSign sums up which videos were watched to their end as phones were last told (phoneSyncPublishWatched); lockSign sums up which known libraries are password-protected or locked on the desktop (phoneSyncPublishLocks).
	watchedSign uint64
	lockSign    uint64
	// unlocked is the password-protected libraries each paired device unlocked for its own session, by device ID then cleaned library folder (state_phonesync_libraries.go); unlockFails tracks recent wrong-password timestamps per device.
	unlocked    map[string]map[string]bool
	unlockFails map[string][]time.Time
	// activity is what the phones did since Kando opened, the latest first (phoneLog); handoffClosed names the place a phone left a film whose offer to continue here the user closed (phoneHandoffClose); readOnlyErr and resolveErr are why the last change of a phone's read-only switch, or the last conflict settled on the page, failed (state_phonesync_linked.go).
	activity                []settings.PhoneActivity
	handoffClosed           string
	readOnlyErr, resolveErr string
	// clockAt is when the timer phoneClockArm set runs the bindings again, as what the page or the handoff bar shows changes with time alone; zero for none, and clockStop stops it.
	clockAt   time.Time
	clockStop func() bool
	// films is the films shared through the storage relay (state_phonesync_films.go).
	films phoneFilms
}

func newPhoneUI() phoneUI { return phoneUI{} }

// savedPhoneSync is Phone Sync's part of settings.json: the switch, the port, what the desktop does with a link a phone sends, and whether the storage relay runs.
type savedPhoneSync struct {
	PhoneSync       bool `json:"phoneSync,omitempty"`
	PhoneSyncPort   int  `json:"phoneSyncPort,omitempty"`
	PhoneReceiveOff bool `json:"phoneReceiveOff,omitempty"`
	PhoneReceiveAsk bool `json:"phoneReceiveAsk,omitempty"`
	PhoneRelay      bool `json:"phoneRelay,omitempty"`
}

// clean keeps a port a server can listen on, and forgets any other.
func (s *savedPhoneSync) clean() {
	if s.PhoneSyncPort < 1024 || s.PhoneSyncPort > 65535 {
		s.PhoneSyncPort = 0
	}
}

// phoneSyncHooks: the switch, the port and what to do with a phone's links are saved and taken back; a switch left on starts the server once the window is ready, and the server stops as the app quits. After each change the code on offer follows the page (phoneSyncFollow), and the server is told the jobs phones follow (phoneSyncPublish) whether the open library's learning state changed (phoneSyncPublishState), and whether a film was watched to its end (phoneSyncPublishWatched).
func (a *app) phoneSyncHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedPhoneSync = savedPhoneSync{PhoneSync: a.phone.on, PhoneSyncPort: a.phone.port, PhoneReceiveOff: a.phone.receiveOff, PhoneReceiveAsk: a.phone.receiveAsk, PhoneRelay: a.phone.relay}
		},
		restore: func(saved savedSettings) {
			a.phone.on, a.phone.port = saved.PhoneSync, saved.PhoneSyncPort
			a.phone.receiveOff, a.phone.receiveAsk = saved.PhoneReceiveOff, saved.PhoneReceiveAsk
			a.phone.relay = saved.PhoneRelay
		},
		ready: func() {
			if a.phone.on {
				a.phoneSyncStart()
			}
		},
		afterChange: func() bool {
			a.phoneSyncPublish()
			a.phoneSyncPublishState()
			a.phoneSyncPublishWatched()
			a.phoneSyncPublishLocks()
			a.phoneClockArm()
			stopped := a.phoneRelayFollow()
			return a.phoneSyncFollow() || stopped
		},
		quitting: a.phoneSyncStop,
	}
}

// phoneSyncPublishWatched tells the phones waiting for events to list the open library's films again once a video was watched to its end, or is no longer (films.WatchState), as a phone goes on with a work at the film after one watched here (psync.Film.Watched), and the learning state's positions drop a film watched to its end. Where a film stopped changes nothing here; the learning state tells that. It runs after every change of the app's state, and changes nothing the window reads.
func (a *app) phoneSyncPublishWatched() {
	srv := a.phone.server
	if srv == nil {
		return
	}
	sign := a.phoneWatchedSign()
	if sign == a.phone.watchedSign {
		return
	}
	a.phone.watchedSign = sign
	if dir := a.snap().LibraryDir; dir != "" && !a.lockShut() && !a.lockFilesAway(dir) {
		srv.PublishFilms(phoneLibraryID(dir))
	}
}

// phoneWatchedSign sums up which videos are currently watched to their end, independent of map order.
func (a *app) phoneWatchedSign() uint64 {
	var sign uint64
	for path := range a.palette.furthest {
		if films.WatchState(a, films.Film{VideoPath: path}) == sidebar.WatchWatched {
			h := fnv.New64a()
			_, _ = h.Write([]byte(path))
			// Each video is summed apart, so the map's order does not count.
			sign += h.Sum64()
		}
	}
	return sign
}

// libraryLocked reports whether the library at dir is locked (LibraryLocked, state_lock.go), which Phone Sync never serves, nor lists.
func (a *app) libraryLocked(dir string) bool { return a.LibraryLocked(dir) }

// phoneSyncDir is Kando's config folder, beside settings.json, where the server keeps its certificate and the paired devices; "" in a test that has no settings file.
func (a *app) phoneSyncDir() string {
	if a.settingsPath == "" {
		return ""
	}
	return filepath.Dir(a.settingsPath)
}

// phoneSyncSet turns Phone Sync on or off, and saves the switch.
func (a *app) phoneSyncSet(on bool) {
	a.phone.on = on
	if on {
		a.phoneSyncStart()
	} else {
		a.phoneSyncStop()
	}
	a.Save()
}

// phoneSyncStart starts the server off the UI thread: reading or making the certificate, and listening, touch the disk and the network.
func (a *app) phoneSyncStart() {
	if a.phone.server != nil || a.phone.starting {
		return
	}
	a.phone.gen++
	a.phone.starting, a.phone.err = true, ""
	gen, update := a.phone.gen, a.update
	name := phoneSyncName()
	a.phone.relaySig, a.phone.relayOK, a.phone.relayErr = a.phoneRelaySig(), false, nil
	cfg := phonesync.Config{
		Dir:           a.phoneSyncDir(),
		Name:          name,
		Port:          a.phone.port,
		Source:        phoneSource{a},
		Jobs:          phoneJobs{a},
		State:         phoneState{a},
		Settings:      phoneSettings{a},
		Study:         phoneStudy{a},
		Subscriptions: phoneSubscriptions{a},
		Glossary:      phoneGlossary{a},
		FilmOps:       phoneFilmOps{a},
		Marks:         phoneMarks{a},
		Watched:       a.phoneWatched,
		Libraries:     phoneLibraries{a},
		Relay:         a.phoneRelayBucket(),
		RelayHealth:   a.phoneRelayHealth(gen),
		Films:         a.phoneFilmsConfig(gen, a.currentStore()),
		Addrs:         a.phone.listen,
		Changed:       func() { update(a.phoneSyncPull) },
		Now:           a.phone.now,
	}
	go func() {
		srv, err := phonesync.Start(cfg)
		update(func() {
			if gen != a.phone.gen {
				if srv != nil {
					go srv.Close()
				}
				return
			}
			a.phone.starting = false
			if err != nil {
				a.phone.err = err.Error()
				log.Printf("phonesync: %v", err)
				return
			}
			a.phone.server, a.phone.running, a.phone.fingerprint, a.phone.name = srv, true, srv.Fingerprint(), name
			a.phone.watchedSign = a.phoneWatchedSign()
			a.phone.lockSign = a.phoneLockSign()
			if port := srv.Port(); port != 0 && port != a.phone.port {
				a.phone.port = port
				a.Save()
			}
			a.phoneSyncPull()
			a.phoneSyncPublish()
		})
	}()
}

// phoneSyncStop stops the server, with its connections, and forgets what the page showed of it; a start under way is dropped when it lands.
func (a *app) phoneSyncStop() {
	a.phone.gen++
	a.phone.starting = false
	if srv := a.phone.server; srv != nil {
		a.phone.server = nil
		if err := srv.Close(); err != nil {
			log.Printf("phonesync: stop: %v", err)
		}
	}
	a.phoneStateKeepAll()
	a.phoneClearUnlocked("")
	clear(a.phone.unlockFails)
	a.lockSettle()
	a.phone.running, a.phone.fingerprint, a.phone.shown, a.phone.watchedSign, a.phone.lockSign = false, nil, false, 0, 0
	a.phone.offer, a.phone.addrs, a.phone.devices, a.phone.revokeErr = phonesync.Offer{}, nil, nil, ""
	a.phone.readOnlyErr, a.phone.resolveErr = "", ""
	a.phone.relaySig, a.phone.relayOK, a.phone.relayErr = "", false, nil
	a.phoneSyncQR()
}

// phoneSyncFollow offers a new code as the page shows with the server running, and takes the code back as the page closes or goes to another section, so a code is never on offer with nobody looking at it. It reports whether it changed what the page reads.
func (a *app) phoneSyncFollow() bool {
	showing := a.phone.server != nil && a.prefs.open && a.SettingsSection() == settings.SectionPhone
	if showing == a.phone.shown {
		return false
	}
	a.phone.shown = showing
	if showing {
		a.phoneSyncNewCode()
	} else if a.phone.server != nil {
		a.phone.server.ClearOffer()
		a.phoneSyncPull()
	}
	return true
}

// phoneSyncNewCode offers a new pairing code in place of the one shown.
func (a *app) phoneSyncNewCode() {
	if a.phone.server == nil {
		return
	}
	a.phone.server.NewOffer()
	a.phoneSyncPull()
}

// phoneSyncRevoke unpairs device id.
func (a *app) phoneSyncRevoke(id string) {
	if a.phone.server == nil {
		return
	}
	a.phone.revokeErr = ""
	name, _ := a.phoneDeviceName(id)
	if err := a.phone.server.Revoke(id); err != nil {
		a.phone.revokeErr = err.Error()
	} else {
		a.phoneClearDeviceUnlocked(id)
		a.phoneLog("", settings.ActivityRevoked, name, 0, 0)
	}
	a.phoneSyncPull()
}

// phoneSyncPull copies what the page shows from the server: the code on offer, where it listens and the devices, and makes the QR code again when the pairing URL changed.
func (a *app) phoneSyncPull() {
	srv := a.phone.server
	if srv == nil {
		return
	}
	a.phone.offer = srv.Offer()
	a.phone.addrs = srv.Addrs()
	devices := srv.Devices()
	// A phone listed now that was not is one that just paired. The first pull after a start lists those paired before.
	if a.phone.devices != nil {
		for _, d := range devices {
			if !slices.ContainsFunc(a.phone.devices, func(o phonesync.Device) bool { return o.ID == d.ID }) {
				a.phoneLog(d.Name, settings.ActivityPaired, "", 0, 0)
			}
		}
	}
	if devices == nil {
		devices = []phonesync.Device{}
	}
	a.phone.devices = devices
	a.phoneSyncQR()
}

// phoneSyncQR makes the QR code of the pairing URL for the code on offer, when it pairs and there is an address to reach; it keeps the one it made while the URL stays the same, so the page's picture is not loaded again.
func (a *app) phoneSyncQR() {
	if !a.phone.running || a.phone.offer.State != phonesync.OfferLive || len(a.phone.addrs) == 0 {
		a.phone.qr, a.phone.qrModules, a.phone.qrText = nil, 0, ""
		return
	}
	text := psync.PairURL{Addrs: a.phone.addrs, Code: a.phone.offer.Code, Fingerprint: a.phone.fingerprint, Name: a.phone.name, Relay: a.phone.offer.Relay}.String()
	if text == a.phone.qrText {
		return
	}
	png, modules, err := phonesync.QRCode(text)
	if err != nil {
		log.Printf("phonesync: QR code: %v", err)
		a.phone.qr, a.phone.qrModules, a.phone.qrText = nil, 0, ""
		return
	}
	a.phone.qr, a.phone.qrModules, a.phone.qrText = png, modules, text
}

// phoneSyncStatus is what the 手机同步 page shows.
func (a *app) phoneSyncStatus() settings.PhoneSync {
	st := settings.PhoneSync{On: a.phone.on, Receive: !a.phone.receiveOff, AskFirst: a.phone.receiveAsk, Running: a.phone.running, Err: a.phone.err, RevokeErr: a.phone.revokeErr, QR: a.phone.qr, QRModules: a.phone.qrModules, Relay: a.phoneRelayStatus()}
	if !a.phone.running {
		return st
	}
	st.Fingerprint = psync.ShortFingerprint(a.phone.fingerprint)
	for _, addr := range a.phone.addrs {
		st.Addrs = append(st.Addrs, addr.String())
	}
	if o := a.phone.offer; o.State != phonesync.OfferNone {
		st.Code, st.Used, st.Expired = o.Code, o.State == phonesync.OfferUsed, o.State == phonesync.OfferExpired
		st.PairAway = o.Relay != nil
	}
	now := a.phoneStateNow()
	for _, dev := range a.phone.devices {
		st.Devices = append(st.Devices, settings.PhoneDevice{ID: dev.ID, Name: dev.Name, Platform: dev.Platform, Paired: dev.Paired, LastSeen: dev.LastSeen, Online: dev.Online(now), ReadOnly: dev.ReadOnly})
	}
	a.phoneSyncLinked(&st)
	return st
}

// phoneSyncName is the computer's name as a phone lists the desktop: its host name, without the .local macOS gives it.
func phoneSyncName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Kando"
	}
	return strings.TrimSuffix(name, ".local")
}

// phoneLibraryID is the protocol's ID of the library at dir.
func phoneLibraryID(dir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(dir)))
	return hex.EncodeToString(sum[:8])
}

// phoneFilmID is the protocol's ID of a film: its job's ID, which stays when its video moves, or for a video of the folder no job holds, "v" and a hash of its path, which a job's ID, 16 hex digits, never is.
func phoneFilmID(item films.Film) string {
	if item.JobID != "" {
		return item.JobID
	}
	sum := sha256.Sum256([]byte(filepath.Clean(item.VideoPath)))
	return "v" + hex.EncodeToString(sum[:8])
}

// phoneSource is the app's libraries as the phone sync server reads them: what the app holds is read on the engine's goroutine (remote.OnUI), the folders and files off it.
type phoneSource struct{ a *app }

// phoneShelf is what the app holds of a library's films, read on the engine's goroutine.
type phoneShelf struct {
	jobs []core.Job
	// added is the videos added to the library without a job (shelf.Library.Added).
	added     map[string]shelf.Added
	names     map[string]string
	durations map[string]time.Duration
	posterDir string
	// furthest is how far each video was played, and played when it last played, by path, which say whether a film was watched to its end and when (films.WatchState).
	furthest map[string]time.Duration
	played   map[string]time.Time
	// stored is where each video moved to a storage location is (shelf.Library.Stored), targets the locations of those that are still in the settings, by ID, and pool what opens them (state_stored.go).
	stored  map[string]shelf.Stored
	targets map[string]storage.Target
	pool    *storage.Pool
	// linked is where the video of each film indexed from a media source is (shelf.Library.Linked), sources the IDs of the sources still in the settings and sourceList those sources, and sourcePool what reads them (state_mediasource_import.go); offline is offline mode as it is when a phone reads, which keeps a source off the local network closed (localHost judging that in place of mediasource.Local when set, as a test's does).
	linked     map[string]shelf.Linked
	sources    map[string]bool
	sourceList []mediasource.Source
	sourcePool *mediasource.Pool
	offline    *atomic.Bool
	localHost  func(ctx context.Context, host string) (bool, error)
	// tag is the interface's language, which orders the films' names (films.Next), and nexts the film after each film of the library, by key, made once the library is listed.
	tag   string
	nexts map[string]films.Film
	// redo is the jobs that process or translate a done film again (core.Job.Redo), by ID.
	redo map[string]bool
	// works is the library's works (films.Works), worked out only as its films are listed (phoneSource.Films), and apart the keys of the works the user split apart (category.Book.ApartKeys): the open library's as the app holds them when apartRead, else read from its folder as the films are listed.
	works     films.WorkIndex
	apart     []string
	apartRead bool
}

// Duration, ResumeAt and Furthest are the shelf as films.Progress, for films.WatchState: where a film stopped does not tell whether it was watched to its end.
func (s phoneShelf) Duration(path string) time.Duration { return s.durations[path] }

func (s phoneShelf) ResumeAt(string) time.Duration { return 0 }

func (s phoneShelf) Furthest(path string) time.Duration { return s.furthest[path] }

// Libraries are the libraries the app knows, named as the library drop-down names them, with Locked and HasPassword set for the calling device (phonesync.DeviceOf).
func (s phoneSource) Libraries(ctx context.Context) ([]phonesync.Library, error) {
	a := s.a
	dev := phonesync.DeviceOf(ctx)
	return remote.OnUI(ctx, a.update, func() ([]phonesync.Library, error) {
		dirs := a.knownLibraries()
		labels := films.LibraryLabels(a.messages(), dirs)
		out := []phonesync.Library{}
		for i, dir := range dirs {
			locked := a.phoneLibraryLocked(dir, dev)
			out = append(out, phonesync.Library{ID: phoneLibraryID(dir), Name: labels[i], Dir: dir, Locked: locked, HasPassword: a.LibraryHasPassword(dir)})
		}
		return out, nil
	})
}

// Films are lib's films that are done, as its done grid shows them, with their files (phoneSource.items), those that keep no video among them (films.Film.SubtitlesOnly), which the server serves by their subtitles. A library locked since it was listed, or whose folder cannot be read, is not found.
func (s phoneSource) Films(ctx context.Context, lib phonesync.Library) ([]phonesync.Film, error) {
	all, shelf, err := s.listed(ctx, lib)
	if err != nil {
		return nil, err
	}
	// The works are made of all of the library's films, as the library page's are, and worked out here, as only the films are told them, apart from the open library's (WorksCache), which is the UI thread's alone.
	shelf.works = films.Works(all, lib.Dir, shelf.tag)
	if !shelf.apartRead {
		// Categories that cannot be read split no work apart, rather than keep the library from the phones.
		book, _ := category.Load(lib.Dir)
		shelf.apart = book.ApartKeys()
	}
	items := phoneDoneOf(all, shelf)
	var out []phonesync.Film
	for _, item := range items {
		f := phoneFilmOf(item, shelf)
		// The film after it, among all of the library's, made or not, as the player goes on (films.Next); the phone plays it once it keeps it.
		if next, ok := shelf.nexts[item.Key]; ok {
			f.Next = phoneFilmID(next)
		}
		// The work the library page folds it into, as the control socket names it (remoteFilmOf), so a phone folds the same films; one the user split apart is none.
		if w, ok := shelf.works.Of(item.Key); ok && !slices.Contains(shelf.apart, w.ApartKey(lib.Dir)) {
			f.Work, f.WorkTitle, f.WorkIndex = w.ID(lib.Dir), w.Name, slices.Index(w.Films, item.Key)+1
		}
		out = append(out, f)
	}
	return out, nil
}

// phoneFilmOf is item, a done film of a library, as Phone Sync serves it, with what shelf holds of it and its files.
func phoneFilmOf(item films.Film, shelf phoneShelf) phonesync.Film {
	f := phonesync.Film{ID: phoneFilmID(item), Title: remoteFilmTitle(item), Source: films.SourceOf(item), Spoken: films.Heard(item), Untranslated: item.Untranslated,
		Duration: shelf.durations[item.VideoPath], Video: item.VideoPath, Base: item.FileBase(), SubtitlesOnly: item.SubtitlesOnly, Sound: !item.SubtitlesOnly && media.IsSoundFile(item.VideoPath), Subtitles: item.SubtitlePath, Transcript: media.TranscriptOf(item.FileBase()), Corrections: media.CorrectionsPath(item.FileBase())}
	// A film that was not translated, as one heard in the language its subtitles are for, names no translation.
	if item.TranslationErr == "" && !item.SameLanguage {
		f.Translation = item.Language
	}
	// A video moved to a storage location is served from there, by ranges as a file here is; one whose location is no longer in the settings is served by its lines alone, as a film that keeps no video is, which a phone cannot download.
	if s, ok := shelf.stored[item.VideoPath]; ok {
		if t, there := shelf.targets[s.Location]; there {
			f.Remote, f.RemoteSize, f.RemoteAt = shelf.pool.Handler(t, s.Path), s.Size, s.Moved
		} else {
			f.SubtitlesOnly = true
		}
	}
	// A film indexed from a media source is served from there alike, the computer reading the source as the phone downloads (docs/media-sources.md 9), with the size and time of change the source gave as it was indexed, or as the file was taken anew (linkedKeepChanged, mediaSourceRelinkTo), so a phone sees a file changed there as changed; when it was indexed for a source that gives no time; once its source is no longer in the settings, by its lines alone.
	if l, ok := shelf.linked[item.VideoPath]; ok {
		if shelf.sources[l.Source] {
			at := l.Modified
			if at.IsZero() {
				at = l.Added
			}
			f.Remote, f.RemoteSize, f.RemoteAt = mediaSourcePhoneHandler(shelf, l), l.Size, at
		} else {
			f.SubtitlesOnly = true
		}
	}
	f.SourceKey = phoneSourceKey(item, shelf)
	if dest := posterFor(shelf.posterDir, item.VideoPath, item.Home); dest != "" && fileExists(dest) {
		f.Poster = dest
	}
	f.Watched, f.PlayedAt = films.WatchState(shelf, item) == sidebar.WatchWatched, shelf.played[item.VideoPath]
	return f
}

// phoneSourceKey names the file of a media source item was made of, so a phone that reaches the same NAS itself takes item's lines for it (psync.Film.SourceKey, docs/media-sources.md 10.5): an indexed film's by the size its source gave as it was indexed, a copied one's by the size of its video here, or of the one moved to a storage location; "" for a film of no source in the settings, of a kind a phone does not reach, or whose video is gone.
func phoneSourceKey(item films.Film, shelf phoneShelf) string {
	if l, ok := shelf.linked[item.VideoPath]; ok {
		return mediasource.SourceKey(shelf.sourceList, mediasource.Ref{Source: l.Source, Path: l.Path}, l.Size)
	}
	ref, err := mediasource.ParseRef(item.Source)
	if err != nil {
		return ""
	}
	size := int64(-1)
	if st, ok := shelf.stored[item.VideoPath]; ok {
		size = st.Size
	} else if info, err := os.Stat(item.VideoPath); err == nil && info.Mode().IsRegular() {
		size = info.Size()
	}
	return mediasource.SourceKey(shelf.sourceList, ref, size)
}

// items are lib's films that are done (phoneDone), as the app files them, with what the app holds of them (listed). A library locked since it was listed, or whose folder cannot be read, is not found.
func (s phoneSource) items(ctx context.Context, lib phonesync.Library) ([]films.Film, phoneShelf, error) {
	all, shelf, err := s.listed(ctx, lib)
	if err != nil {
		return nil, phoneShelf{}, err
	}
	return phoneDoneOf(all, shelf), shelf, nil
}

// phoneDoneOf is those of all that are done (phoneDone).
func phoneDoneOf(all []films.Film, shelf phoneShelf) []films.Film {
	var out []films.Film
	for _, item := range all {
		if phoneDone(item, shelf) {
			out = append(out, item)
		}
	}
	return out
}

// listed is every film of lib as the app files it: the jobs the shelf keeps for it and the videos in its folder (films.List), with what the app holds of them. A library locked since it was listed, or whose folder cannot be read, is not found.
func (s phoneSource) listed(ctx context.Context, lib phonesync.Library) ([]films.Film, phoneShelf, error) {
	a := s.a
	dev := phonesync.DeviceOf(ctx)
	shelf, err := remote.OnUI(ctx, a.update, func() (phoneShelf, error) {
		if a.phoneLibraryLocked(lib.Dir, dev) {
			return phoneShelf{}, phonesync.ErrNotFound
		}
		// The open library's split works as the app holds them, which a save that failed may leave apart from its file; another's are read from its file below.
		var apart []string
		open := a.cats.book.Dir() == lib.Dir
		if open {
			apart = a.cats.book.ApartKeys()
		}
		return phoneShelf{apart: apart, apartRead: open, jobs: slices.Clone(a.library.Jobs), added: maps.Clone(a.library.AddedIn(lib.Dir)), names: maps.Clone(a.library.Names), durations: maps.Clone(a.lib.durations), posterDir: a.lib.posterDir,
			furthest: maps.Clone(a.palette.furthest), played: maps.Clone(a.player.played),
			stored: maps.Clone(a.library.Stored), targets: a.storedPlayable(), pool: a.stored.pool,
			linked: maps.Clone(a.library.Linked), sources: a.mediaSourceIDs(), sourceList: slices.Clone(a.sources.list), sourcePool: a.sources.pool, offline: a.privacy.now, localHost: a.sources.localHost, tag: a.prefs.interfaceLanguage}, nil
	})
	if err != nil {
		return nil, phoneShelf{}, err
	}
	// The videos moved to a storage location and those indexed from a media source are the library's as the app files them (awayPaths), so the phone is told the films the library page shows, and the works and the film after each made of them.
	away := slices.Sorted(slices.Values(slices.Concat(slices.Collect(maps.Keys(shelf.stored)), slices.Collect(maps.Keys(shelf.linked)))))
	videos, err := films.Videos(lib.Dir, shelf.jobs, away, shelf.added)
	if err != nil {
		return nil, phoneShelf{}, phonesync.ErrNotFound
	}
	noDetail := func(core.Job) string { return "" }
	all := linkedMark(storedMark(films.List(lib.Dir, shelf.jobs, videos, shelf.names, jobName, noDetail), shelf.stored), shelf.linked)
	shelf.nexts = films.Nexts(all, shelf.tag)
	shelf.redo = map[string]bool{}
	for _, job := range shelf.jobs {
		if job.Redo != "" {
			shelf.redo[job.ID] = true
		}
	}
	return all, shelf, nil
}

// phoneDone reports whether item is a done film, as a phone lists it: one whose job is done, or that no job processed, and one done before that is processed or translated again (core.Job.Redo), which keeps the video and lines it had until the job writes the new ones, as the player shows it meanwhile, so it stays on the phones meanwhile.
func phoneDone(item films.Film, shelf phoneShelf) bool {
	return item.VideoPath != "" && (!films.InQueue(item) || shelf.redo[item.JobID])
}

// phoneMaking reports whether item is a film being made, as a phone lists it to watch while it is made (psync.Film.Making): one whose job waits or runs for the first time, as the player opens it from its card (films.Openable); a done film made again is done (phoneDone).
func phoneMaking(item films.Film, shelf phoneShelf) bool {
	return films.Openable(item) && !shelf.redo[item.JobID]
}

// Making are lib's films being made (phoneMaking), with their files as they are so far: the video, or a link's sound until its video comes (core.Job.SoundPath, phonesync.Film.ComingSound), none before either is in; the film, its lines, corrections and poster go by where its video is or will be (films.PlayPath), as the player keeps its place by it. A library locked since it was listed, or whose folder cannot be read, is not found.
func (s phoneSource) Making(ctx context.Context, lib phonesync.Library) ([]phonesync.Film, error) {
	all, shelf, err := s.listed(ctx, lib)
	if err != nil {
		return nil, err
	}
	var out []phonesync.Film
	for _, item := range all {
		if !phoneMaking(item, shelf) {
			continue
		}
		at := films.PlayPath(item)
		f := phonesync.Film{ID: phoneFilmID(item), Title: remoteFilmTitle(item), Source: films.SourceOf(item), Spoken: films.Heard(item), Making: true,
			Duration: shelf.durations[at], Video: at, Subtitles: item.SubtitlePath}
		if item.VideoPath == "" {
			f.ComingSound = item.SoundPath
		}
		if at != "" {
			f.Base = core.FileBase(at, item.Home)
			f.Transcript, f.Corrections = media.TranscriptOf(f.Base), media.CorrectionsPath(f.Base)
			if dest := posterFor(shelf.posterDir, at, item.Home); dest != "" && fileExists(dest) {
				f.Poster = dest
			}
		}
		if !item.SameLanguage {
			f.Translation = item.Language
		}
		out = append(out, f)
	}
	return out, nil
}

// Cues are f's subtitles as the control socket reads them (remoteFilmCues): its lines as its subtitle document holds them, each original split from its translation by the original .srt. A film without subtitles is not found.
func (s phoneSource) Cues(_ context.Context, f phonesync.Film) ([]psync.Cue, error) {
	rf := remote.Film{Title: f.Title, VideoPath: f.Video, FileBase: cmp.Or(f.Base, f.Video), Subtitles: f.Subtitles}
	if fileExists(f.Transcript) {
		rf.OriginalSubtitles = f.Transcript
	}
	cues, err := remoteFilmCues(rf)
	if err != nil {
		var e *remote.Error
		if errors.As(err, &e) && e.Code == remote.CodeNotFound {
			return nil, phonesync.ErrNotFound
		}
		return nil, err
	}
	out := make([]psync.Cue, len(cues))
	for i, c := range cues {
		out[i] = psync.Cue{StartMS: c.StartMS, EndMS: c.EndMS, Original: c.Text, Translation: c.Translation}
	}
	return out, nil
}
