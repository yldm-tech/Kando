package main

import (
	"os"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The app's screens as pictures, which go tool nsgo test -target ios_simulator (or android) -run TestScreenshots -env KANDO_SCREENSHOTS=1 saves in .nsgo/test/screenshots/<target>/<lang>/<scene>.png; headless each scene plays its steps and checks what it shows, drawing nothing. They are skipped without KANDO_SCREENSHOTS, as the desktop app's are.
func TestScreenshots(t *testing.T) {
	if os.Getenv("KANDO_SCREENSHOTS") == "" {
		t.Skip("KANDO_SCREENSHOTS is not set")
	}
	shot := func(p *phone, scene string) {
		t.Helper()
		p.app.Screenshot(lang + "/" + scene)
	}
	t.Run("welcome", func(t *testing.T) {
		p := startPhone(t, "")
		p.app.WaitFor("id:home-welcome", wait)
		shot(p, "welcome")
	})
	t.Run("pair", func(t *testing.T) {
		d := newDesktop(t)
		p := startPhone(t, "")
		p.app.WaitFor("id:home-welcome", wait)
		p.app.Press("id:home-pair")
		p.fillPairing(d.Addr.String(), "123456")
		shot(p, "pair")
		p.app.Press("id:pair-continue")
		p.app.WaitFor("id:pair-fingerprint", wait)
		shot(p, "pair-compare")
	})
	t.Run("browse", func(t *testing.T) {
		d := newDesktop(t)
		p := startPhone(t, "")
		p.openTalk(d)
		d.Stall(420_000)
		p.app.Press("id:film-download")
		p.app.WaitFor("text:"+msgs.FilmDownloading(42), wait)
		shot(p, "film-downloading")
		p.app.HeaderBack("id:film-page")
		p.app.WaitFor("id:library-film-f1", wait)
		shot(p, "library")
		p.app.HeaderBack("id:library-page")
		p.app.WaitFor("id:home-film-f1", wait)
		shot(p, "home")
	})
	t.Run("player", func(t *testing.T) {
		p, _ := openPlayer(t)
		p.at(3200 * time.Millisecond)
		p.app.WaitFor("id:player-subtitles >> text:你好吗？", wait)
		shot(p, "player")
		p.app.Resize(844, 390)
		shot(p, "player-landscape")
	})
	t.Run("tablet", func(t *testing.T) {
		p, _ := openPlayer(t, nsgotest.Size(1180, 820))
		p.at(3200 * time.Millisecond)
		p.app.WaitFor("id:player-subtitles >> text:你好吗？", wait)
		shot(p, "player-tablet")
	})
}
