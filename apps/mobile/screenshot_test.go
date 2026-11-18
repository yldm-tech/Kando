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
	shot(p, "onboarding-languages")
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-pairing", wait)
	p.fillPairing(d.Addr.String(), "123456")
	shot(p, "onboarding-pair")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-fingerprint", wait)
	shot(p, "onboarding-pair-compare")
	p.app.Press("id:pair-confirm")
	p.app.WaitFor("id:onboarding-paired", wait)
	shot(p, "onboarding-paired")
}

func TestScreenshotsSettings(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:computers-library-lib", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-computer-"+store.DesktopID(d.Fingerprint), wait)
	shot(p, "settings")
	p.app.Press("id:settings-target")
	p.app.WaitFor("id:settings-target-page", wait)
	shot(p, "settings-target")
}

func TestScreenshotsLearn(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:downloads-film-f1", wait)
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
	p.app.WaitFor("id:computers-library-lib", wait)
	// A target reports its own idiom, which an app on a computer's or a phone's is not a tablet's.
	if !p.app.Exists("id:sidebar") {
		t.Skip("the target is not a tablet")
	}
	p.app.WaitFor("id:computers-library-lib", wait)
	shot(p, "tablet-library")
	p.app.Resize(820, 1180)
	p.app.WaitFor("id:tab-bar", wait)
	p.app.WaitFor("id:computers-library-lib", wait)
	shot(p, "tablet-library-upright")
}

func TestScreenshotsBrowse(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	p := startPhone(t, t.TempDir())
	p.pair(d)
	p.app.WaitFor("id:computers-library-lib", wait)
	shot(p, "films")
	p.app.Press("id:computers-library-lib")
	p.app.WaitFor("id:library-film-f1", wait)
	shot(p, "library")
	// Forward only: HeaderBack reports what the platform would, which a phone skips.
	p.app.Press("id:library-film-f1")
	p.app.WaitFor("id:film-download", wait)
	d.Stall(420_000)
	p.app.Press("id:film-download")
	p.app.WaitFor("text:"+msgs.FilmDownloading(42), wait)
	// The bar eases to its value over 200ms (ui.Progress).
	p.app.Advance(300 * time.Millisecond)
	shot(p, "film-downloading")
	// Paused, so the stalled answer ends before the desktop closes: on a phone the app outlives the test's cleanups.
	p.app.Press("id:film-pause")
	p.app.WaitFor("id:film-resume", wait)
}

func TestScreenshotsDownloads(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:downloads-film-f1", wait)
	shot(p, "films-unpaired")
	p.app.Press("id:downloads-film-f1")
	p.app.WaitFor("id:film-play", wait)
	shot(p, "film-downloaded")
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

func TestScreenshotsLibraryLoading(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	// The phone reaches the desktop through a gate, held once the libraries are listed so the library's films stay loading.
	g := newGate(t, d)
	desktops, err := store.Open(dir).Desktops()
	if err != nil || len(desktops) != 1 {
		t.Fatalf("kept %+v, %v", desktops, err)
	}
	desktops[0].Addr = g.addr
	if err := store.Open(dir).SaveDesktops(desktops); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:computers-library-lib", wait)
	g.held.Store(true)
	p.app.Press("id:computers-library-lib")
	p.app.WaitFor("id:library-loading", wait)
	shot(p, "library-loading")
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

func TestScreenshotsTablet(t *testing.T) {
	shot := screenshots(t)
	p, _ := openPlayer(t, nsgotest.Size(1180, 820))
	p.at(3200 * time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-card-translation") == "你好吗？" })
	shot(p, "player-tablet")
}

func TestScreenshotsPlayerStudy(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetEntry("morning", sync.WordEntry{Word: "morning", Reading: "/ˈmɔːnɪŋ/", Meaning: "n. 早晨；上午。这里和 good 连用，是早上见面时的问候。", Language: "简体中文"})
	d.SetExplanation("整句是早上见面时的问候，相当于「早上好」。\n\nGood morning 是固定说法，不说 good morning to you 也同样自然。")
	// Without reporting the video's progress, which a target cannot, the card shows the first line.
	p, _ := openStudy(t, d)
	shot(p, "player-card")
	p.app.Press("id:player-save")
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-text", wait)
	shot(p, "player-explain")
	p.app.Press("id:player-explain-close")
	p.app.Press("id:player-tab-notes")
	p.app.WaitFor("id:player-saved-line", wait)
	shot(p, "player-notes")
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-follow")
	p.app.Press("id:player-settings-sleep-15")
	p.app.WaitFor("id:player-settings", wait)
	shot(p, "player-settings")
}
