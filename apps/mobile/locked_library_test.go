package main

import (
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
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
