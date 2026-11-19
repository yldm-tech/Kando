package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
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
	// activity is what the phones did since Kando opened, the latest first (phoneLog); handoffClosed names the place a phone left a film whose offer to continue here the user closed (phoneHandoffClose); readOnlyErr and resolveErr are why the last change of a phone's read-only switch, or the last conflict settled on the page, failed (state_phonesync_linked.go).
	activity                []settings.PhoneActivity
	handoffClosed           string
	readOnlyErr, resolveErr string
	// clockAt is when the timer phoneClockArm set runs the bindings again, as what the page or the handoff bar shows changes with time alone; zero for none, and clockStop stops it.
	clockAt   time.Time
	clockStop func() bool
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

// phoneSyncHooks: the switch, the port and what to do with a phone's links are saved and taken back; a switch left on starts the server once the window is ready, and the server stops as the app quits. After each change the code on offer follows the page (phoneSyncFollow), and the server is told the jobs phones follow (phoneSyncPublish) and whether the open library's learning state changed (phoneSyncPublishState).
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
			a.phoneClockArm()
			stopped := a.phoneRelayFollow()
			return a.phoneSyncFollow() || stopped
		},
		quitting: a.phoneSyncStop,
	}
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
		Films:         phoneFilms{a},
		Relay:         a.phoneRelayBucket(),
		RelayHealth:   a.phoneRelayHealth(gen),
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
	a.phone.running, a.phone.fingerprint, a.phone.shown = false, nil, false
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
	text := psync.PairURL{Addrs: a.phone.addrs, Code: a.phone.offer.Code, Fingerprint: a.phone.fingerprint, Name: a.phone.name}.String()
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
	jobs      []core.Job
	names     map[string]string
	durations map[string]time.Duration
	posterDir string
}

// Libraries are the libraries the app knows, named as the library drop-down names them, locked ones left out.
func (s phoneSource) Libraries(ctx context.Context) ([]phonesync.Library, error) {
	a := s.a
	return remote.OnUI(ctx, a.update, func() ([]phonesync.Library, error) {
		dirs := a.knownLibraries()
		labels := films.LibraryLabels(a.messages(), dirs)
		out := []phonesync.Library{}
		for i, dir := range dirs {
			if !a.libraryLocked(dir) {
				out = append(out, phonesync.Library{ID: phoneLibraryID(dir), Name: labels[i], Dir: dir})
			}
		}
		return out, nil
	})
}

// Films are lib's films that are done, as its done grid shows them, with their files (phoneSource.items), those that keep no video among them (films.Film.SubtitlesOnly), which the server serves by their subtitles. A library locked since it was listed, or whose folder cannot be read, is not found.
func (s phoneSource) Films(ctx context.Context, lib phonesync.Library) ([]phonesync.Film, error) {
	items, shelf, err := s.items(ctx, lib)
	if err != nil {
		return nil, err
	}
	var out []phonesync.Film
	for _, item := range items {
		out = append(out, phoneFilmOf(item, shelf))
	}
	return out, nil
}

// phoneFilmOf is item, a done film of a library, as Phone Sync serves it, with what shelf holds of it and its files.
func phoneFilmOf(item films.Film, shelf phoneShelf) phonesync.Film {
	f := phonesync.Film{ID: phoneFilmID(item), Title: remoteFilmTitle(item), Source: films.SourceOf(item), Spoken: films.Heard(item), Untranslated: item.Untranslated,
		Duration: shelf.durations[item.VideoPath], Video: item.VideoPath, SubtitlesOnly: item.SubtitlesOnly, Subtitles: item.SubtitlePath, Transcript: media.TranscriptOf(item.VideoPath), Corrections: media.CorrectionsPath(item.VideoPath)}
	// A film that was not translated, as one heard in the language its subtitles are for, names no translation.
	if item.TranslationErr == "" && !item.SameLanguage {
		f.Translation = item.Language
	}
	if dest := posterDest(shelf.posterDir, item.VideoPath); dest != "" && fileExists(dest) {
		f.Poster = dest
	}
	return f
}

// items are lib's films that are done, as the app files them: the jobs the shelf keeps for it and the videos in its folder (films.List), with what the app holds of them. A library locked since it was listed, or whose folder cannot be read, is not found.
func (s phoneSource) items(ctx context.Context, lib phonesync.Library) ([]films.Film, phoneShelf, error) {
	a := s.a
	shelf, err := remote.OnUI(ctx, a.update, func() (phoneShelf, error) {
		if a.libraryLocked(lib.Dir) {
			return phoneShelf{}, phonesync.ErrNotFound
		}
		return phoneShelf{jobs: slices.Clone(a.library.Jobs), names: maps.Clone(a.library.Names), durations: maps.Clone(a.lib.durations), posterDir: a.lib.posterDir}, nil
	})
	if err != nil {
		return nil, phoneShelf{}, err
	}
	videos, err := media.ListVideos(lib.Dir)
	if err != nil {
		return nil, phoneShelf{}, phonesync.ErrNotFound
	}
	noDetail := func(core.Job) string { return "" }
	var out []films.Film
	for _, item := range films.List(lib.Dir, shelf.jobs, videos, shelf.names, jobName, noDetail) {
		if !films.InQueue(item) && item.VideoPath != "" {
			out = append(out, item)
		}
	}
	return out, shelf, nil
}

// Cues are f's subtitles as the control socket reads them (remoteFilmCues): each line of the speech from the original .srt, with its translation from the bilingual one. A film without subtitles is not found.
func (s phoneSource) Cues(_ context.Context, f phonesync.Film) ([]psync.Cue, error) {
	rf := remote.Film{Title: f.Title, Subtitles: f.Subtitles}
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
