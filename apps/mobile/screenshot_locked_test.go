package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs/davtest"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The Library of a phone that downloaded films of the computer's library Private before it was locked, beside a film of the NAS Home NAS: nothing of Private's films shows, and the NAS film's row names the NAS (locked-library). Then Learn's saved lines, which show none from Private (locked-learn).
func TestScreenshotsLockedLibrary(t *testing.T) {
	shot := screenshots(t)
	d := lockedDesktop(t)
	dir := t.TempDir()
	secrets := pairedTokens(t, dir, d)
	id := store.DesktopID(d.Fingerprint)
	secret := keepSecret(t, dir, id)
	st := store.Open(dir)
	writePoster(t, filepath.Join(st.FilmDir(id, secret.Film.ID), secret.Poster))
	src := davSource(davFilms(t, false))
	if err := st.SaveSources([]store.Source{src}); err != nil {
		t.Fatal(err)
	}
	secrets[store.SourceSecretAccount(src.ID, nas.Password)] = davtest.Password
	f := store.Film{Desktop: store.SourceOwner(src.ID), Library: src.Name, Source: src.ID, Path: "Holiday.mp4", Video: "video.mp4", Added: now().Add(-time.Hour),
		Film: sync.Film{ID: store.SourceFilmID("Holiday.mp4"), Title: "Holiday", Source: "local", VideoSize: int64(len(talkVideo))}}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, nsgotest.Secrets(secrets))
	p.app.WaitFor(talkRow(id), wait)
	p.app.WaitFor(filmRow(f.Desktop, f.Film.ID), wait)
	p.app.Advance(time.Second)
	shot(p, "locked-library")
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-line-l-talk", wait)
	p.learnScrollTo("id:learn-lines")
	shot(p, "locked-learn")
}
