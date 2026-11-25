package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs/davtest"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// sceneSources keeps in dir the two sources the scenes show, which they never connect to: a NAS's SMB share and a server over SFTP, by names written here.
func sceneSources(t *testing.T, dir string) {
	t.Helper()
	onboarded(t, dir)
	list := []store.Source{
		{ID: "1a2b3c4d5e6f7081", Kind: nas.KindSMB, Name: "Living Room NAS", Config: map[string]string{nas.Host: "nas.local", nas.Share: "media", nas.Root: "Films", nas.User: "family"}},
		{ID: "8f7e6d5c4b3a2910", Kind: nas.KindSFTP, Name: "Studio Server", Config: map[string]string{nas.Host: "studio.lan", nas.Root: "/srv/films", nas.User: "kando"}},
	}
	if err := store.Open(dir).SaveSources(list); err != nil {
		t.Fatal(err)
	}
}

// The Library with the NAS group, over a film downloaded from the NAS among those on the phone.
func TestScreenshotsSourcesLibrary(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	sceneSources(t, dir)
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:films-sources", wait)
	shot(p, "sources-library")
}

// Settings with its NAS & Servers row, and that page with the two sources.
func TestScreenshotsSourcesSettings(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	sceneSources(t, dir)
	p := startPhone(t, dir)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-sources", wait)
	p.app.Scroll("id:settings-scroll", 5000)
	shot(p, "sources-settings-row")
	p.app.Press("id:settings-sources")
	p.app.WaitFor("id:sources-source-1a2b3c4d5e6f7081", wait)
	shot(p, "sources-settings")
}

// The form adding an SMB share, the NAS doing without signing, which it warns of.
func TestScreenshotsSourcesAdd(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	onboarded(t, dir)
	p := startPhone(t, dir)
	p.openSettingsSources()
	p.app.Press("id:sources-add")
	p.app.WaitFor("id:source-field-host", wait)
	p.app.Type("id:source-name", "Living Room NAS")
	p.app.Type("id:source-field-host", "nas.local")
	p.app.Type("id:source-field-share", "media")
	p.app.Type("id:source-field-user", "family")
	// The switch is at the form's foot, which shows the warnings once it is on.
	p.app.Scroll("id:source-scroll", 5000)
	p.app.Press("id:source-field-noSigning")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("text:" + msgs.SourceNoSigningRisk()) })
	p.app.Scroll("id:source-scroll", 5000)
	shot(p, "sources-add-bottom")
	p.app.Scroll("id:source-scroll", -5000)
	shot(p, "sources-add")
}

// A folder of a WebDAV source: a folder, a video downloaded and two not.
func TestScreenshotsSourcesBrowse(t *testing.T) {
	shot := screenshots(t)
	f := davFilms(t, false)
	for name, size := range map[string]int{"Films/Lecture on Rivers.mp4": 2 << 20, "Films/Weekend Market.mkv": 3 << 20} {
		if err := os.WriteFile(filepath.Join(f.Dir, name), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	src := davSource(f)
	opt := keepSource(t, dir, src, davtest.Password)
	st := store.Open(dir)
	id := store.SourceFilmID("Holiday.mp4")
	rec := store.Film{Desktop: store.SourceOwner(src.ID), Library: src.Name, Source: src.ID, Path: "Holiday.mp4", Video: "video.mp4", Added: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	rec.Film.ID, rec.Film.Title, rec.Film.VideoSize, rec.Film.Source = id, "Holiday", int64(len(talkVideo)), "local"
	if err := st.SaveFilm(rec); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(rec.Desktop, id), rec.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, opt, nsgotest.PendingAlerts())
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-downloaded-Holiday.mp4", wait)
	p.app.WaitFor("text:Weekend Market.mkv", wait)
	shot(p, "sources-browse")
}

// A video of a WebDAV source streamed into the player as its row is pressed, then, the NAS switched off, the player saying it cannot play it from there and why. On a simulator the system's player really plays the talk's animation from the phone's loopback server, which the scene logs: what it asked the NAS for, and the time left before and after a few seconds of playing.
func TestScreenshotsSourcesStream(t *testing.T) {
	shot := screenshots(t)
	f := davFilms(t, false)
	if err := os.WriteFile(filepath.Join(f.Dir, "Films/Morning Talk.mp4"), storeAsset(t, "talk.mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := davSource(f)
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password))
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-video-Morning Talk.mp4", wait)
	f.ForgetRanges()
	p.app.Press("id:browse-video-Morning Talk.mp4")
	p.app.WaitFor("id:player-video", wait)
	// On a phone the system's player reads the talk's length from the loopback server, then plays it on: the time played moves. Headless no player opens the URL.
	if runtime.GOOS == "ios" || runtime.GOOS == "android" {
		p.app.WaitUntil(wait, func() bool { return len(f.Ranges()) > 0 && p.texts("id:player-left") == "-0:10" })
		p.app.Press("id:player-play")
		p.app.WaitUntil(wait, func() bool { return p.texts("id:player-at") != "0:00" })
		p.app.Press("id:player-play")
		t.Logf("the player played %s from the loopback server, asking the NAS for %q; time left %q", p.playerSource(), f.Ranges(), p.texts("id:player-left"))
	}
	shot(p, "sources-stream")

	f.Close()
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:browse-video-Holiday.mp4", wait)
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitUntil(wait, func() bool { return strings.HasSuffix(p.playerSource(), "/Holiday.mp4") })
	// What the system's player asks first, which headless nothing asks.
	fetchRange(t, p.playerSource(), 0, 9)
	p.app.WaitFor("id:player-failed-why", wait)
	shot(p, "sources-stream-unreachable")
}
