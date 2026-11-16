package main

import (
	"os"
	"testing"
	"time"

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

func TestScreenshotsWelcome(t *testing.T) {
	shot := screenshots(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	shot(p, "welcome")
}

func TestScreenshotsPair(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	p.app.Press("id:home-pair")
	p.fillPairing(d.Addr.String(), "123456")
	shot(p, "pair")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-fingerprint", wait)
	shot(p, "pair-compare")
}

func TestScreenshotsBrowse(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	// TODO(nsgo#475): paired beforehand, as popping the pairing page crashes on iOS; pair through the page again once a popped input no longer crashes https://github.com/nsgo-dev/nsgo/issues/475
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:home-library-lib", wait)
	shot(p, "home")
	p.app.Press("id:home-library-lib")
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

func TestScreenshotsPlayer(t *testing.T) {
	shot := screenshots(t)
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	p.app.WaitFor("id:player-subtitles >> text:你好吗？", wait)
	shot(p, "player")
	p.app.Resize(844, 390)
	shot(p, "player-landscape")
}

func TestScreenshotsTablet(t *testing.T) {
	shot := screenshots(t)
	p, _ := openPlayer(t, nsgotest.Size(1180, 820))
	p.at(3200 * time.Millisecond)
	p.app.WaitFor("id:player-subtitles >> text:你好吗？", wait)
	shot(p, "player-tablet")
}
