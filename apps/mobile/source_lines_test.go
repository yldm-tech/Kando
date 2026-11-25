package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"
	"kando/packages/remotefs/davtest"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// holidayKey is the key a computer names Holiday.mp4 of src by (sync.Film.SourceKey), reaching the same server at host in place of src's.
func holidayKey(t *testing.T, src store.Source, host string) string {
	t.Helper()
	r := nas.Remote(src, nil)
	r.URL = strings.Replace(r.URL, "127.0.0.1", host, 1)
	key := remotefs.SourceKey(r, "Holiday.mp4", int64(len(talkVideo)))
	if key == "" {
		t.Fatalf("no key for %+v", r)
	}
	return key
}

// holidayDesktop is a desktop named Studio offering code 123456 whose library lists a film of Holiday.mp4 made on it, named by key, with the talk's lines.
func holidayDesktop(t *testing.T, key string) *synctest.Desktop {
	t.Helper()
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese Class")
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Cues: talkCues,
		Film: sync.Film{ID: "f2", Title: "Holiday on the NAS", Source: "local", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: now(), SourceKey: key}})
	d.Offer("123456")
	return d
}

// startWithSourceAndDesktop starts a phone in dir that has src, with its password, and is paired with d.
func startWithSourceAndDesktop(t *testing.T, dir string, src store.Source, d *synctest.Desktop) *phone {
	t.Helper()
	onboarded(t, dir)
	if err := store.Open(dir).SaveSources([]store.Source{src}); err != nil {
		t.Fatal(err)
	}
	secrets := pairedTokens(t, dir, d)
	secrets[store.SourceSecretAccount(src.ID, nas.Password)] = davtest.Password
	return startPhone(t, dir, nsgotest.Secrets(secrets))
}

// playsHolidayFromTheSource presses Holiday.mp4 in the folder of src, once the Library lists the computer's films, and checks that the player streams its video from the source.
func playsHolidayFromTheSource(t *testing.T, p *phone, src store.Source, desktop string) {
	t.Helper()
	p.app.WaitFor(filmRow(desktop, "f2"), wait)
	p.openSource(src.ID)
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	if link := p.playerSource(); !strings.HasPrefix(link, "http://127.0.0.1:") || !strings.HasSuffix(link, "/Holiday.mp4") {
		t.Fatalf("the player plays %q", link)
	}
}

// A video of a NAS that a paired computer made a film of plays from the NAS with the computer's lines: the phone knows the computer's film by the key of the file (sync.Film.SourceKey), takes its lines into that film's folder, which is no film of the Library, and keeps which film it took them from with the film of the NAS.
func TestASourceFilmTakesTheComputersLines(t *testing.T) {
	dir := t.TempDir()
	src := davSource(davFilms(t, false))
	d := holidayDesktop(t, holidayKey(t, src, "127.0.0.1"))
	desktop := store.DesktopID(d.Fingerprint)
	p := startWithSourceAndDesktop(t, dir, src, d)
	playsHolidayFromTheSource(t, p, src, desktop)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") })
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(1500 * sec / 1000)
	if got := p.texts("id:player-card-original"); got != "Good morning." {
		t.Fatalf("the card shows %q", got)
	}
	if got := p.texts("id:player-card-translation"); got != "早上好。" {
		t.Fatalf("the card's translation is %q", got)
	}
	key := store.Key(store.SourceOwner(src.ID), store.SourceFilmID("Holiday.mp4"))
	var kept store.Film
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(dir).Films()
		for _, f := range films {
			if f.Key() == key && f.Lines != nil {
				kept = f
				return true
			}
		}
		return false
	})
	if l := kept.Lines; l.Desktop != desktop || l.Library != "lib" || l.LibraryName != "Japanese Class" || l.Film.ID != "f2" || !l.Film.Updated.Equal(now()) {
		t.Fatalf("the film keeps its lines from %+v", l)
	}
	linesDir := store.Open(dir).FilmDir(desktop, "f2")
	if _, err := os.Stat(filepath.Join(linesDir, "subtitles.json")); err != nil {
		t.Fatalf("the lines are not kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(linesDir, "film.json")); err == nil {
		t.Fatal("the computer's film is kept as a film of the Library")
	}
	// Only the film of the NAS is on the phone; the computer's is listed as the computer's.
	if films, _ := store.Open(dir).Films(); len(films) != 1 {
		t.Fatalf("the phone keeps %d films", len(films))
	}
}

// A computer that names the NAS otherwise, as by its name where the phone has its address, makes no film of the same file on purpose: the film of the NAS plays without lines, as it did before.
func TestASourceFilmOfAServerNamedOtherwiseHasNoLines(t *testing.T) {
	dir := t.TempDir()
	src := davSource(davFilms(t, false))
	d := holidayDesktop(t, holidayKey(t, src, "localhost"))
	desktop := store.DesktopID(d.Fingerprint)
	p := startWithSourceAndDesktop(t, dir, src, d)
	playsHolidayFromTheSource(t, p, src, desktop)
	filmJSON := filepath.Join(dir, "films", store.SourceOwner(src.ID), store.SourceFilmID("Holiday.mp4"), "film.json")
	p.app.WaitUntil(wait, func() bool { _, err := os.Stat(filmJSON); return err == nil })
	if p.app.Exists("id:player-line") {
		t.Fatal("the film shows lines")
	}
	films, err := store.Open(dir).Films()
	if err != nil || len(films) != 1 || films[0].Lines != nil {
		t.Fatalf("the phone keeps %+v, %v", films, err)
	}
	if _, err := os.Stat(store.Open(dir).FilmDir(desktop, "f2")); err == nil {
		t.Fatal("the computer's film's lines were taken")
	}
}

// A film of a NAS that took a computer's lines shows them where the computer cannot be reached, as it is not paired any more: they are kept on the phone, the Library lists the film as the computer's it took them from, and a search finds them. Deleted from the phone, the film takes them with it.
func TestASourceFilmShowsItsLinesOffline(t *testing.T) {
	dir := t.TempDir()
	src := davSource(davFilms(t, false))
	st := store.Open(dir)
	f := store.Film{Desktop: store.SourceOwner(src.ID), Library: src.Name, Source: src.ID, Path: "Holiday.mp4", Video: "video.mp4", Added: now(),
		Film:  sync.Film{ID: store.SourceFilmID("Holiday.mp4"), Title: "Holiday", Source: "local", VideoSize: int64(len(talkVideo))},
		Lines: &store.Lines{Desktop: "gonedesk", Library: "lib", LibraryName: "Japanese Class", Film: sync.Film{ID: "f2", Title: "Holiday on the NAS", Spoken: "ja", Translation: "简体中文", Updated: now()}}}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues("gonedesk", "f2", talkCues); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password))
	row := filmRow("gonedesk", "f2")
	p.app.WaitFor(row, wait)
	p.app.Type("id:films-search", "HOW ARE")
	hit := "id:films-hit-gonedesk-f2"
	p.app.WaitFor(hit, wait)
	if got, want := p.texts(hit), msgs.FilmsHit("0:03", "How are you?"); got != want {
		t.Fatalf("the hit says %q, want %q", got, want)
	}
	p.app.Clear("id:films-search")
	p.app.WaitFor(row, wait)
	p.app.Press(row)
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") })
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(3500 * sec / 1000)
	if got := p.texts("id:player-card-original"); got != "How are you?" {
		t.Fatalf("the card shows %q", got)
	}
	if got := p.texts("id:player-card-translation"); got != "你好吗？" {
		t.Fatalf("the card's translation is %q", got)
	}
	p.app.Press("id:player-minimize")
	options := "id:films-options-gonedesk-f2"
	p.app.WaitFor(options, wait)
	p.app.Press(options)
	p.choose(msgs.FilmDelete())
	p.app.Answer(1)
	p.waitFile(filepath.Join("films", f.Desktop, f.Film.ID), -1)
	p.waitFile(filepath.Join("films", "gonedesk", "f2"), -1)
}

// Away from the computer's network, a film of the NAS takes the computer's lines out of the user's storage, where the computer shares its films (sync.RelayShelf), as a computer's film is downloaded from there.
func TestASourceFilmTakesTheComputersLinesFromItsStorage(t *testing.T) {
	dir := t.TempDir()
	src := davSource(davFilms(t, false))
	d := holidayDesktop(t, holidayKey(t, src, "127.0.0.1"))
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	d.ShareFilms(true)
	desktop := store.DesktopID(d.Fingerprint)
	p := startWithSourceAndDesktop(t, dir, src, d)
	folder := sync.RelayPrefix(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 && len(bucket.Keys(folder+"outbox/")) > 0 })
	p.waitRelayJoined(desktop)
	p.app.WaitUntil(wait, func() bool {
		return slices.ContainsFunc(bucket.Keys(folder+"films/"), func(k string) bool { return strings.HasSuffix(k, "/"+sync.RelaySubtitlesFile) })
	})

	d.SetOff(true)
	playsHolidayFromTheSource(t, p, src, desktop)
	p.app.WaitUntil(reconnect, func() bool { return p.app.Exists("id:player-line") })
	if _, ok, _ := store.Open(dir).Cues(desktop, "f2"); !ok {
		t.Fatal("the lines are not kept")
	}
	if !slices.ContainsFunc(bucket.Gets(), func(g string) bool { return strings.Contains(g, "/"+sync.RelaySubtitlesFile+" ") }) {
		t.Fatalf("the storage answered %q", bucket.Gets())
	}
}
