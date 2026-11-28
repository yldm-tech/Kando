package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/apps/mobile/study"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A locked library on the computer appears in the Library switcher with its lock status; tapping it prompts for its password, shows an inline error on a wrong password, switches directly into the unlocked library on the right password, and lets the user lock it back from the switcher.
func TestMobileShowsLockedLibraryAndUnlocksWithPassword(t *testing.T) {
	d := newDesktop(t)
	d.AddLockedLibrary("priv", "Private", "secret")
	d.AddFilm(synctest.Film{Library: "priv", Video: talkVideo, Cues: talkCues,
		Film: sync.Film{ID: "secret-film", Title: "Secret Documentary", Source: "local", DurationMS: 60_000, Spoken: "ja", Updated: now()}})
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)

	p.app.WaitFor(talkRow(id), wait)
	if p.app.Exists(filmRow(id, "secret-film")) {
		t.Fatal("film of a locked library showed before unlocking")
	}

	// Open switcher: Private appears with Locked indicator.
	p.app.Press("id:films-library")
	p.app.WaitFor("id:films-library-option-2", wait)
	if got := p.texts("id:films-library-option-2"); !strings.Contains(got, "Private") || !strings.Contains(got, msgs.FilmsLibraryLocked()) {
		t.Fatalf("locked library row says %q, want Private + %q", got, msgs.FilmsLibraryLocked())
	}

	// Tapping the locked library opens the unlock sheet.
	p.app.Press("id:films-library-option-2")
	p.app.WaitFor("id:films-unlock-sheet", wait)

	// Wrong password shows inline error and keeps the sheet open.
	p.app.Type("id:films-unlock-password", "wrong")
	p.app.Press("id:films-unlock-confirm")
	p.app.WaitFor("id:films-unlock-error", wait)
	if got := p.texts("id:films-unlock-error"); got != msgs.FilmsUnlockWrong() {
		t.Fatalf("wrong password error = %q, want %q", got, msgs.FilmsUnlockWrong())
	}

	// Right password closes the unlock sheet, switches straight to Private, and shows its film.
	p.app.Clear("id:films-unlock-password")
	p.app.Type("id:films-unlock-password", "secret")
	p.app.Press("id:films-unlock-confirm")
	p.app.WaitForGone("id:films-unlock-sheet", wait)
	p.app.WaitFor(filmRow(id, "secret-film"), wait)
	if got := p.texts("id:films-library"); got != "Private" {
		t.Fatalf("switcher says %q, want Private", got)
	}
	if p.app.Exists(talkRow(id)) {
		t.Fatal("normal library film still shown while scoped to Private")
	}

	// Reopen switcher and lock Private back via its trailing lock button.
	p.app.Press("id:films-library")
	p.app.WaitFor("id:films-library-lock-priv", wait)
	p.app.Press("id:films-library-lock-priv")
	p.app.WaitForGone("id:films-library-lock-priv", wait)
	p.app.Press("id:films-library-option-0")
	p.app.WaitForGone("id:films-library-sheet", wait)
	p.app.WaitFor(talkRow(id), wait)
	if p.app.Exists(filmRow(id, "secret-film")) {
		t.Fatal("film of Private still shown after locking it")
	}
}

// When the computer locks a library again (LockNow / EventFilms), an open phone immediately drops its films and leaves the library scope.
func TestMobileFallsBackWhenDesktopLocksLibrary(t *testing.T) {
	d := newDesktop(t)
	d.AddLockedLibrary("priv", "Private", "secret")
	d.AddFilm(synctest.Film{Library: "priv", Video: talkVideo, Cues: talkCues,
		Film: sync.Film{ID: "secret-film", Title: "Secret Documentary", Source: "local", DurationMS: 60_000, Spoken: "ja", Updated: now()}})
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)

	p.app.WaitFor(talkRow(id), wait)
	p.app.Press("id:films-library")
	p.app.WaitFor("id:films-library-option-2", wait)
	p.app.Press("id:films-library-option-2")
	p.app.WaitFor("id:films-unlock-sheet", wait)
	p.app.Type("id:films-unlock-password", "secret")
	p.app.Press("id:films-unlock-confirm")
	p.app.WaitFor(filmRow(id, "secret-film"), wait)

	// Desktop locks Private: mobile receives EventFilms, drops the film, and falls back to All Libraries.
	d.SetLibraryUnlocked("priv", false)
	p.app.WaitForGone(filmRow(id, "secret-film"), wait)
	p.app.WaitFor(talkRow(id), wait)
	if got := p.texts("id:films-library"); got != msgs.FilmsAllLibraries() {
		t.Fatalf("after desktop lock, switcher says %q, want %q", got, msgs.FilmsAllLibraries())
	}
}

// secretCues are the lines of Secret Documentary, the film of the tests' locked library.
var secretCues = []sync.Cue{{StartMS: 1000, EndMS: 3000, Original: "The vault opens at dawn.", Translation: "金库黎明时打开。"}}

// lockedDesktop is the tests' desktop with, beside the talk's library, the password-protected library Private, locked, holding Secret Documentary.
func lockedDesktop(t *testing.T) *synctest.Desktop {
	t.Helper()
	d := newDesktop(t)
	d.AddLockedLibrary("priv", "Private", "secret")
	d.AddFilm(synctest.Film{Library: "priv", Video: talkVideo, Poster: []byte("jpeg"), Cues: secretCues,
		Film: sync.Film{ID: "secret-film", Title: "Secret Documentary", Source: "local", DurationMS: 60_000, Spoken: "ja", Updated: now()}})
	return d
}

// keepSecret puts on the phone what it downloaded from computer id before its library Private was locked: Secret Documentary, left partway, with its video, poster and lines, and a line saved from it that the computer has not taken yet, which keeps the library's learning state on the phone while it is locked; and the talk of Japanese Class with a line saved from it the same way.
func keepSecret(t *testing.T, dir, id string) store.Film {
	t.Helper()
	st := store.Open(dir)
	keepTalkFrom(t, dir, id)
	f := store.Film{Desktop: id, Library: "Private", LibraryID: "priv", Video: "video.mp4", Poster: "poster.jpg", Added: now(),
		PositionMS: 2000, PositionAt: now().Add(-time.Hour),
		Film: sync.Film{ID: "secret-film", Title: "Secret Documentary", DurationMS: 60_000, Spoken: "ja", VideoSize: int64(len(talkVideo))}}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(id, f.Film.ID, secretCues); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{f.Video: talkVideo, f.Poster: []byte("jpeg")} {
		if err := os.WriteFile(filepath.Join(st.FilmDir(id, f.Film.ID), name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	saved := func(lib, line, film, title string, c sync.Cue) {
		r := study.Save(line, film, title, study.Cue{Start: time.Duration(c.StartMS) * time.Millisecond, End: time.Duration(c.EndMS) * time.Millisecond, Original: c.Original, Translation: c.Translation}, "", now().Add(-2*time.Hour))
		if err := st.SaveLearning(id, lib, study.Library{Records: []sync.Record{r}, Pending: []sync.Record{r}}); err != nil {
			t.Fatal(err)
		}
	}
	saved("priv", "l-secret", "secret-film", "Secret Documentary", secretCues[0])
	saved("lib", "l-talk", "f1", "Morning Talk", talkCues[0])
	return f
}

// The films the phone downloaded from a library before it was locked show nothing of it while it is locked: not in the Library's list, Continue Watching, a search of its titles or its lines, Settings › Offline Films, or Learn's saved lines, and they stay on the phone. The phone keeps the library as locked, so they stay hidden as it starts again away from the computer. Unlocked on the phone, they all come back; locked again on the computer, the film playing closes and they go again.
func TestALockedLibrarysFilmsOnThePhoneShowNothingUntilUnlocked(t *testing.T) {
	d := lockedDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	secret := keepSecret(t, dir, id)
	p := startPhone(t, dir, secrets)
	row, card := filmRow(id, "secret-film"), "id:films-continue-"+id+"-secret-film"

	// The talk comes from the computer's listing, which names Private as locked.
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists(talkRow(id)) && !p.app.Exists(row) && !p.app.Exists(card)
	})
	p.app.Advance(time.Second)
	if p.app.Exists(row) || p.app.Exists(card) {
		t.Fatal("a film of the locked library shows in the Library")
	}
	for _, q := range []string{"Secret", "vault"} {
		p.app.Type("id:films-search", q)
		p.app.WaitFor("id:films-empty", wait)
		if p.app.Exists(row) || p.app.Exists("id:films-hit-"+id+"-secret-film") {
			t.Fatalf("a search for %q finds the locked library's film", q)
		}
		p.app.Clear("id:films-search")
	}
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-line-l-talk", wait)
	if p.app.Exists("id:learn-line-l-secret") {
		t.Fatal("Learn shows a line saved from the locked library's film")
	}
	p.openTab(settingsTab, msgs.SettingsTitle())
	if got, want := p.texts("id:settings-offline"), msgs.SettingsOffline()+"\n"+msgs.SettingsOfflineCount(1); got != want {
		t.Fatalf("the Offline Films row reads %q, want %q", got, want)
	}
	p.app.Press("id:settings-offline")
	p.app.WaitFor("id:offline-film-"+store.Key(id, "f1"), wait)
	if p.app.Exists("id:offline-film-" + secret.Key()) {
		t.Fatal("Offline Films lists the locked library's film")
	}
	films, err := store.Open(dir).Films()
	if err != nil || !slices.ContainsFunc(films, func(f store.Film) bool { return f.Key() == secret.Key() }) {
		t.Fatalf("the locked library's film is gone from the phone: %v %v", films, err)
	}
	locked, _ := store.Open(dir).Locked()
	if got := locked[id]; len(got) != 1 || got[0].ID != "priv" {
		t.Fatalf("the phone keeps %v locked, want Private", got)
	}

	// Unlocked on the phone, its film, its place to go on from and its line come back.
	p.app.Back()
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.Press("id:films-library")
	p.app.WaitFor("id:films-library-option-2", wait)
	p.app.Press("id:films-library-option-2")
	p.app.WaitFor("id:films-unlock-sheet", wait)
	p.app.Type("id:films-unlock-password", "secret")
	p.app.Press("id:films-unlock-confirm")
	p.app.WaitFor(row, wait)
	p.app.WaitFor(card, wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-line-l-secret", wait)

	// Locked again on the computer, the film playing closes and the film goes again.
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.Press(card)
	p.app.WaitFor("id:player-video", wait)
	d.SetLibraryUnlocked("priv", false)
	p.app.WaitForGone("id:player-video", wait)
	p.app.WaitForGone(row, wait)
	if p.app.Exists(card) {
		t.Fatal("Continue Watching shows the film of the library locked again")
	}
}

// A phone that knows a library locked keeps its films hidden as it starts again away from the computer, which cannot tell it.
func TestALockedLibrarysFilmsStayHiddenAwayFromTheComputer(t *testing.T) {
	d := lockedDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	d.Close()
	keepSecret(t, dir, id)
	if err := store.Open(dir).SaveLocked(map[string][]store.LockedLibrary{id: {{ID: "priv", Name: "Private"}}}); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, secrets)
	p.app.WaitFor(talkRow(id), wait)
	p.app.Advance(time.Second)
	if p.app.Exists(filmRow(id, "secret-film")) || p.app.Exists("id:films-continue-"+id+"-secret-film") {
		t.Fatal("a film of the locked library shows while the computer is away")
	}
}

// The widgets do not show a locked library's film to go on with, or count the lines saved from it as due, so the Lock Screen shows nothing of it.
func TestTheWidgetsShowNothingOfALockedLibrary(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	d := lockedDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepSecret(t, dir, id)
	p := startPhone(t, dir, secrets, nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS}))
	p.app.WaitFor(talkRow(id), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-widgets", wait)
	p.app.Press("id:settings-widgets")
	p.app.WaitFor("id:settings-widgets-page", wait)
	p.app.WaitFor("id:settings-widget-film-none", wait)
	p.app.Advance(time.Second)
	if !p.app.Exists("id:settings-widget-film-none") {
		t.Fatal("the Continue widget shows the locked library's film")
	}
	if got := p.app.Find("id:settings-widget-lock-due").Text; got != "1" {
		t.Fatalf("the Lock Screen's ring shows %q due, want the talk's line alone", got)
	}
}
