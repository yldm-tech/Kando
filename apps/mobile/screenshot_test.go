package main

import (
	"io"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The app's screens as pictures, which go tool nsgo test -target ios_simulator (or android) -run TestScreenshots -env KANDO_SCREENSHOTS=1 saves in .nsgo/test/screenshots/<target>/<lang>/<scene>.png; headless each scene plays its steps and checks what it shows, drawing nothing. They are skipped without KANDO_SCREENSHOTS, as the desktop app's are. Each starts one app in a test of its own: a phone runs each test in a process of its own and shows only its first app, skipping a test that starts a second.

// screenshots skips t without KANDO_SCREENSHOTS, and returns what saves p's screen as scene.
func screenshots(t *testing.T) func(p *phone, scene string) {
	t.Helper()
	if os.Getenv("KANDO_SCREENSHOTS") == "" {
		t.Skip("KANDO_SCREENSHOTS is not set")
	}
	return func(p *phone, scene string) {
		t.Helper()
		p.app.Screenshot(lang + "/" + scene)
	}
}

// TODO(nsgo#948): onboarding-pair shows no Scan QR Code: a target reports its own camera, none on macOS or a simulator, whatever the test's Environment says; add a scene of the pairing step with Environment{Camera: true} once a target takes it https://github.com/nsgo-dev/nsgo/issues/948
func TestScreenshotsOnboarding(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:onboarding-intro", wait)
	shot(p, "onboarding-intro")
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-languages", wait)
	p.app.Press("id:onboarding-spoken-ja")
	p.app.Press("id:onboarding-spoken-en")
	p.app.Press("id:onboarding-target")
	p.app.WaitFor("id:onboarding-target-zh-Hans", wait)
	shot(p, "onboarding-target")
	p.app.Press("id:onboarding-target-zh-Hans")
	// The sheet's exit runs on the test's clock.
	p.app.Advance(500 * time.Millisecond)
	p.app.WaitForGone("id:onboarding-target-sheet", wait)
	shot(p, "onboarding-languages")
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-pairing", wait)
	p.fillPairing(d.Addr.String(), "123456")
	shot(p, "onboarding-pair")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-fingerprint", wait)
	shot(p, "onboarding-pair-compare")
	p.app.Press("id:pair-confirm")
	p.app.WaitFor("id:onboarding-paired-films", wait)
	shot(p, "onboarding-paired")
}

func TestScreenshotsSettings(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-computer", wait)
	p.app.WaitFor("text:"+msgs.SettingsComputerOnline(), wait)
	shot(p, "settings")
	p.app.Scroll("id:settings-scroll", 5000)
	shot(p, "settings-bottom")
	p.app.Scroll("id:settings-scroll", -3000)
	// Light, then dark, as Settings' Appearance steps from the system's look.
	p.app.Press("id:settings-appearance")
	p.app.Press("id:settings-appearance")
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-page").Style.Background == core.RGB(0x000000) })
	shot(p, "settings-dark")
	p.app.Press("id:settings-appearance")
	p.app.Press("id:settings-target")
	p.app.WaitFor("id:settings-target-page", wait)
	shot(p, "settings-target")
}

// TestScreenshotsSettingsLanguage is Settings › Language, and TestScreenshotsOfflineFilms Settings › Offline Films with a film on the phone, each in an app of its own, as a target takes no step back.
func TestScreenshotsSettingsLanguage(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-language")
	p.app.WaitFor("id:settings-language-page", wait)
	shot(p, "settings-language")
}

func TestScreenshotsOfflineFilms(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	f := keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-offline")
	p.app.WaitFor("id:offline-film-"+f.Key(), wait)
	shot(p, "settings-offline")
}

func TestScreenshotsLearn(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-empty", wait)
	shot(p, "learn")
}

func TestScreenshotsTabletShell(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	keepTalkFrom(t, dir, store.DesktopID(d.Fingerprint))
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	// A target reports its own idiom, which an app on a computer's or a phone's is not a tablet's.
	if !p.app.Exists("id:sidebar") {
		t.Skip("the target is not a tablet")
	}
	shot(p, "tablet-library")
	p.app.Resize(820, 1180)
	p.app.WaitFor("id:tab-bar", wait)
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	shot(p, "tablet-library-upright")
}

func TestScreenshotsBrowse(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	p := startPhone(t, t.TempDir())
	p.pair(d)
	shot(p, "films")
	// Forward only: HeaderBack reports what the platform would, which a phone skips.
	p.app.Press(talkRow(store.DesktopID(d.Fingerprint)))
	p.app.WaitFor("id:film-download", wait)
	d.Stall(420_000)
	p.app.Press("id:film-download")
	p.app.WaitFor("text:"+msgs.FilmDownloading(42), wait)
	// The bar eases to its value over 200ms (ui.Progress).
	p.app.Advance(300 * time.Millisecond)
	shot(p, "film-downloading")
	// Paused, so the stalled answer ends before the desktop closes: on a phone the app outlives the test's cleanups; then resumed to the end, where the page stays.
	p.app.Press("id:film-pause")
	p.app.WaitFor("id:film-resume", wait)
	d.Stall(0)
	p.app.Press("id:film-resume")
	p.app.WaitFor("id:film-play", wait)
	shot(p, "film-downloaded")
}

func TestScreenshotsDownloads(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	shot(p, "films-unpaired")
}

func TestScreenshotsComputersProblem(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	d.Close()
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:computers-problem-"+store.DesktopID(d.Fingerprint), wait)
	shot(p, "computers-problem")
}

func TestScreenshotsLibrary(t *testing.T) {
	shot := screenshots(t)
	d := newDesktopWithMovies(t)
	running := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 2, 0.84)
	running.Title, running.Device = "Morning Market in Kyoto", ""
	d.SetJob(running)
	moved(d, "job2", "https://youtu.be/ScMzIvxBSi4", sync.JobFailed, 0, 0)
	d.SetState("lib", now().Add(time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(4000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepTalkFrom(t, dir, id)
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:films-continue-"+id+"-f1", wait)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	shot(p, "films-continue")
	p.app.Type("id:films-search", "how")
	p.app.WaitFor("id:films-hit-"+id+"-f1", wait)
	shot(p, "films-search")
}

func TestScreenshotsLibrarySubscriptions(t *testing.T) {
	shot := screenshots(t)
	d := newDesktopWithMovies(t)
	d.AddSubscription(earthScience())
	d.AddSubscription(sync.Subscription{ID: "s2", Library: "movies", Name: "线性代数的几何意义", Kind: sync.SubscriptionCollection, Platform: sync.PlatformBilibili, Auto: true, AddPerCheck: 3})
	d.AddSubscription(sync.Subscription{ID: "s3", Library: "lib", Name: "日语听力每日一练", Kind: sync.SubscriptionCollection, Platform: sync.PlatformBilibili, Gone: true, AddPerCheck: 3})
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor("id:films-subscription-"+id+"-s3", wait)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	shot(p, "films-subscriptions")
	p.app.Press("id:films-subscription-" + id + "-s1")
	p.app.WaitFor("id:films-subscription-auto-switch", wait)
	shot(p, "films-subscription-sheet")
}

func TestScreenshotsLibraryLoading(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	// The phone reaches the desktop through a gate, held from the start so the computer's films stay loading.
	g := newGate(t, d)
	desktops, err := store.Open(dir).Desktops()
	if err != nil || len(desktops) != 1 {
		t.Fatalf("kept %+v, %v", desktops, err)
	}
	desktops[0].Addr = g.addr
	if err := store.Open(dir).SaveDesktops(desktops); err != nil {
		t.Fatal(err)
	}
	g.held.Store(true)
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:films-loading", wait)
	shot(p, "films-loading")
}

// gate is a relay on loopback to a desktop that passes on what the phone sends until held, then holds it, so a request made after hangs as on a network that stalls. It closes as the test ends, which fails what it held.
type gate struct {
	addr netip.AddrPort
	held atomic.Bool
	stop chan struct{}
}

// newGate starts a gate to d.
func newGate(t *testing.T, d *synctest.Desktop) *gate {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &gate{addr: netip.MustParseAddrPort(ln.Addr().String()), stop: make(chan struct{})}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go g.relay(c, d.Addr)
		}
	}()
	t.Cleanup(func() { close(g.stop); ln.Close() })
	return g
}

// relay passes c's bytes to the desktop at to and back, holding what c sends once the gate is held.
func (g *gate) relay(c net.Conn, to netip.AddrPort) {
	defer c.Close()
	up, err := net.Dial("tcp", to.String())
	if err != nil {
		return
	}
	defer up.Close()
	go func() { _, _ = io.Copy(c, up); c.Close() }()
	buf := make([]byte, 32<<10)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			if g.held.Load() {
				<-g.stop
				return
			}
			if _, err := up.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func TestScreenshotsPlayer(t *testing.T) {
	shot := screenshots(t)
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-card-translation") == "你好吗？" })
	shot(p, "player")
	p.app.Resize(844, 390)
	shot(p, "player-landscape")
}

func TestScreenshotsMiniPlayer(t *testing.T) {
	shot := screenshots(t)
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	shot(p, "player-mini")
}

func TestScreenshotsTablet(t *testing.T) {
	shot := screenshots(t)
	p, _ := openPlayer(t, nsgotest.Size(1180, 820))
	p.at(3200 * time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-card-translation") == "你好吗？" })
	shot(p, "player-tablet")
}

func TestScreenshotsPlayerRecap(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetRecap(sync.RecapItem{StartMS: 1000, Text: "两人在早上见面，互相问候。"}, sync.RecapItem{StartMS: 3000, Text: "一方问对方最近过得怎么样。"}, sync.RecapItem{StartMS: 5000, Text: "对方说一切都好，并道谢。"})
	p := openStudyAt(t, d, 6*sec)
	p.app.WaitFor("id:player-recap-note", wait)
	shot(p, "player-recap")
}

// The card of a line of Japanese, whose words the dictionary cut, underlined to tap.
func TestScreenshotsPlayerJapanese(t *testing.T) {
	shot := screenshots(t)
	p, _ := openJapanese(t)
	shot(p, "player-card-japanese")
}

func TestScreenshotsPlayerStudy(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetEntry("morning", sync.WordEntry{Word: "morning", Reading: "/ˈmɔːnɪŋ/", Meaning: "n. 早晨；上午。这里和 good 连用，是早上见面时的问候。", Language: "简体中文", Source: sync.WordSource{Kind: sync.SourceAI, Desktop: "Studio"}})
	d.SetExplanationSections(
		sync.ExplainSection{Kind: sync.ExplainMeaning, Text: "整句是早上见面时的问候，相当于「早上好」。"},
		sync.ExplainSection{Kind: sync.ExplainBreakdown, Text: "good morning · 早上好，早上见面时的固定问候"},
		sync.ExplainSection{Kind: sync.ExplainTone, Text: "轻松、友好，熟人和陌生人之间都能用。"},
	)
	// Without reporting the video's progress, which a target cannot, the card shows the first line.
	p, _ := openStudy(t, d)
	shot(p, "player-card")
	p.app.Press("id:player-save")
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-note", wait)
	shot(p, "player-explain")
	p.app.Press("id:player-explain-more")
	p.app.Type("id:player-explain-ask", "晚上能说吗？")
	p.app.Press("id:player-explain-send")
	p.app.WaitUntil(wait, func() bool {
		return len(p.app.Query("id:player-explain-answer")) == 1 && p.app.Find("id:player-explain-answer").Text != msgs.PlayerExplainAnswering()
	})
	shot(p, "player-explain-follow-up")
	p.app.Press("id:player-explain-close")
	p.app.Press(`id:player-card >> text:"morning"`)
	p.app.WaitFor("id:player-word-vocab", wait)
	p.app.Press("id:player-word-vocab")
	p.app.WaitFor("id:player-word-vocab >> text:"+msgs.PlayerWordInVocabulary(), wait)
	shot(p, "player-word")
	p.app.Press("id:player-word-close")
	p.app.Press("id:player-tab-words")
	p.app.WaitFor("id:player-film-word", wait)
	shot(p, "player-words")
	p.app.Press("id:player-tab-notes")
	p.app.WaitFor("id:player-saved-line", wait)
	shot(p, "player-notes")
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-follow")
	p.app.Press("id:player-settings-sleep-15")
	p.app.WaitFor("id:player-settings", wait)
	shot(p, "player-settings")
}

// TestScreenshotsPlayerGlossary renders the sheet that fixes the first line's translation with 同时加进术语表 on and the term filled in (player-fix), and the Info tab's glossary as the computer keeps it (player-terms).
func TestScreenshotsPlayerGlossary(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetExplanation("整句是早上见面时的问候，相当于「早上好」。")
	d.SetGlossary("lib", "f1", "日常会话", true,
		sync.Term{Source: "Good morning", Target: "早安", Scope: sync.TermFilm, Locked: true},
		sync.Term{Source: "thanks", Target: "谢谢", Scope: sync.TermSeries},
		sync.Term{Source: "See you", Target: "回头见", Scope: sync.TermGlobal})
	p, _ := openStudy(t, d)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-fix", wait)
	p.app.Press("id:player-explain-fix")
	p.app.WaitFor("id:player-fix", wait)
	p.app.Clear("id:player-fix-draft")
	p.app.Type("id:player-fix-draft", "早安。")
	p.app.Press("id:player-fix-term-switch")
	p.app.WaitFor("id:player-fix-term-source", wait)
	p.app.Type("id:player-fix-term-source", "Good morning")
	shot(p, "player-fix")
	p.app.Press("id:player-fix-cancel")
	p.app.Press("id:player-tab-info")
	p.app.WaitFor("id:player-terms", wait)
	shot(p, "player-terms")
}

func TestScreenshotsTabletSubscriptions(t *testing.T) {
	shot := screenshots(t)
	d := newDesktopWithMovies(t)
	d.AddSubscription(earthScience())
	d.AddSubscription(sync.Subscription{ID: "s2", Library: "movies", Name: "线性代数的几何意义", Kind: sync.SubscriptionCollection, Platform: sync.PlatformBilibili, Auto: true, AddPerCheck: 3})
	d.AddSubscription(sync.Subscription{ID: "s3", Library: "lib", Name: "日语听力每日一练", Kind: sync.SubscriptionCollection, Platform: sync.PlatformBilibili, Gone: true, AddPerCheck: 3})
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	// A target reports its own idiom, which an app on a computer's or a phone's is not a tablet's.
	if !p.app.Exists("id:sidebar") {
		t.Skip("the target is not a tablet")
	}
	p.app.WaitFor("id:sidebar-subscription-"+id+"-s3", wait)
	shot(p, "tablet-subscriptions")
	p.app.Press("id:sidebar-subscription-" + id + "-s1")
	p.app.WaitFor("id:films-subscription-auto-switch", wait)
	shot(p, "tablet-subscription-sheet")
}

func TestScreenshotsSubtitlesOnly(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.AddFilm(linesFilm())
	dir := t.TempDir()
	id := store.DesktopID(d.Fingerprint)
	// The film's lines are on the phone already, as Save Lines to Phone keeps them.
	st := store.Open(dir)
	kept := store.Film{Desktop: id, Library: "Japanese Class", LinesOnly: true, Added: now(), Film: linesFilm().Film}
	if err := st.SaveFilm(kept); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(id, kept.Film.ID, talkCues); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(filmRow(id, "f3")+" >> text:YouTube · 0:10 · "+msgs.FilmSubtitlesOnly()+" · "+msgs.FilmLinesSaved(), wait)
	p.app.Press(filmRow(id, "f3"))
	p.app.WaitFor("id:film-no-video-ask", wait)
	shot(p, "film-subtitles-only")
	p.app.Press("id:film-play")
	p.app.WaitFor("id:player-no-video-ask", wait)
	shot(p, "player-subtitles-only")
}
