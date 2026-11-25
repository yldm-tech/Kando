package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs/davtest"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// holidayFilm is the computer's film of Holiday.mp4 that holidayDesktop lists, as the phone keeps it with a film of the NAS that took its lines (store.Lines).
func holidayFilm() sync.Film {
	return sync.Film{ID: "f2", Title: "Holiday on the NAS", Source: "local", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: now()}
}

// keepHoliday puts Holiday.mp4 of src on the phone in dir as a film of the source, as an earlier run kept it: streamed, or downloaded with its video (video), and, unless desktop is "", matched to the film f2 of desktop, whose lines it took into f2's folder (store.Film.Lines).
func keepHoliday(t *testing.T, dir string, src store.Source, desktop string, video bool) store.Film {
	t.Helper()
	st := store.Open(dir)
	f := store.Film{Desktop: store.SourceOwner(src.ID), Library: src.Name, Source: src.ID, Path: "Holiday.mp4", Added: now(), Streamed: !video,
		Film: sync.Film{ID: store.SourceFilmID("Holiday.mp4"), Title: "Holiday", Source: "local", VideoSize: int64(len(talkVideo))}}
	if video {
		f.Video = "video.mp4"
	}
	if desktop != "" {
		f.Lines = &store.Lines{Desktop: desktop, Library: "lib", LibraryName: "Japanese Class", Film: holidayFilm()}
		if err := st.SaveCues(desktop, "f2", talkCues); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if video {
		if err := os.WriteFile(filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Video), talkVideo, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// keepComputerHoliday puts the computer's film f2 of desktop on the phone in dir as downloaded from the computer, with its lines and its video.
func keepComputerHoliday(t *testing.T, dir, desktop string) store.Film {
	t.Helper()
	st := store.Open(dir)
	f := store.Film{Desktop: desktop, Library: "Japanese Class", LibraryID: "lib", Film: holidayFilm(), Video: "video.mp4", Added: now()}
	f.Film.VideoSize = int64(len(talkVideo))
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(desktop, "f2", talkCues); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(desktop, "f2"), f.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

// mergedPhone is a phone that has the WebDAV source of nas and is paired with a desktop listing the film f2 of its Holiday.mp4, with keep putting what the phone kept of it before it starts.
type mergedPhone struct {
	*phone
	nas     *davtest.Server
	src     store.Source
	desktop string
	row     string
}

func startMerged(t *testing.T, keep func(dir string, src store.Source, desktop string)) mergedPhone {
	t.Helper()
	dir := t.TempDir()
	f := davFilms(t, false)
	src := davSource(f)
	d := holidayDesktop(t, holidayKey(t, src, "127.0.0.1"))
	desktop := store.DesktopID(d.Fingerprint)
	keep(dir, src, desktop)
	p := startWithSourceAndDesktop(t, dir, src, d)
	m := mergedPhone{phone: p, nas: f, src: src, desktop: desktop, row: filmRow(desktop, "f2")}
	p.app.WaitFor(m.row, wait)
	return m
}

// nasFilm is the key of the film of Holiday.mp4 of the phone's source.
func (m mergedPhone) nasFilm() string {
	return store.Key(store.SourceOwner(m.src.ID), store.SourceFilmID("Holiday.mp4"))
}

// kept is the film the phone keeps on disk under key.
func (m mergedPhone) kept(key string) (store.Film, bool) {
	films, _ := store.Open(m.dir).Films()
	for _, f := range films {
		if f.Key() == key {
			return f, true
		}
	}
	return store.Film{}, false
}

// A film of the NAS matched to the computer's film of the same file is listed once, as the computer's film, by its title, in the Library and in Offline Films; matched to nothing, the two are listed apart, as before.
func TestAMatchedSourceFilmIsListedOnce(t *testing.T) {
	t.Run("matched", func(t *testing.T) {
		m := startMerged(t, func(dir string, src store.Source, desktop string) { keepHoliday(t, dir, src, desktop, true) })
		p := m.phone
		if p.app.Exists(filmRow(store.SourceOwner(m.src.ID), store.SourceFilmID("Holiday.mp4"))) {
			t.Fatal("the film of the NAS is listed beside the computer's")
		}
		if got := p.texts("id:films-count"); got != msgs.FilmsCount(1) {
			t.Fatalf("the count says %q", got)
		}
		if got := p.texts(m.row); !strings.Contains(got, "Holiday on the NAS") || !strings.Contains(got, msgs.FilmDownloaded()) {
			t.Fatalf("the film's row reads %q", got)
		}
		p.openTab(settingsTab, msgs.SettingsTitle())
		if got, want := p.texts("id:settings-offline"), msgs.SettingsOffline()+"\n"+msgs.SettingsOfflineCount(1); got != want {
			t.Fatalf("the Offline Films row reads %q, want %q", got, want)
		}
		p.app.Press("id:settings-offline")
		p.app.WaitFor("id:offline-film-"+m.nasFilm(), wait)
		if got := p.texts("id:offline-film-" + m.nasFilm()); !strings.HasPrefix(got, "Holiday on the NAS") {
			t.Fatalf("the film's row in Offline Films reads %q", got)
		}
	})
	t.Run("matched to nothing", func(t *testing.T) {
		dir := t.TempDir()
		src := davSource(davFilms(t, false))
		d := holidayDesktop(t, holidayKey(t, src, "localhost"))
		n := keepHoliday(t, dir, src, "", true)
		p := startWithSourceAndDesktop(t, dir, src, d)
		p.app.WaitFor(filmRow(store.DesktopID(d.Fingerprint), "f2"), wait)
		p.app.WaitFor(filmRow(n.Desktop, n.Film.ID), wait)
		if got := p.texts("id:films-count"); got != msgs.FilmsCount(2) {
			t.Fatalf("the count says %q", got)
		}
	})
}

// The matched film plays a video downloaded to the phone before anything over a network: the NAS's download, or the computer's.
func TestAMatchedFilmPlaysADownloadedVideoFirst(t *testing.T) {
	for name, keep := range map[string]func(t *testing.T, dir string, src store.Source, desktop string) string{
		"from the NAS": func(t *testing.T, dir string, src store.Source, desktop string) string {
			n := keepHoliday(t, dir, src, desktop, true)
			return filepath.Join(dir, "films", n.Desktop, n.Film.ID, "video.mp4")
		},
		"from the computer": func(t *testing.T, dir string, src store.Source, desktop string) string {
			keepHoliday(t, dir, src, desktop, false)
			keepComputerHoliday(t, dir, desktop)
			return filepath.Join(dir, "films", desktop, "f2", "video.mp4")
		},
	} {
		t.Run(name, func(t *testing.T) {
			var want string
			m := startMerged(t, func(dir string, src store.Source, desktop string) { want = keep(t, dir, src, desktop) })
			m.app.Press(m.row)
			m.app.WaitFor("id:player-video", wait)
			m.app.WaitUntil(wait, func() bool { return m.playerSource() == want })
			if got := m.texts("id:films-count"); got != msgs.FilmsCount(1) {
				t.Fatalf("the count says %q", got)
			}
		})
	}
}

// With nothing of it on the phone, the matched film streams from the NAS while the NAS answers, though the computer could stream it too.
func TestAMatchedFilmStreamsFromTheNASBeforeTheComputer(t *testing.T) {
	m := startMerged(t, func(dir string, src store.Source, desktop string) { keepHoliday(t, dir, src, desktop, false) })
	m.app.Press(m.row)
	m.app.WaitFor("id:player-video", wait)
	link := m.playerSource()
	if !strings.HasPrefix(link, "http://127.0.0.1:") || !strings.HasSuffix(link, "/Holiday.mp4") {
		t.Fatalf("the player plays %q", link)
	}
	if code, body := fetch(t, link); code != http.StatusOK || body != string(talkVideo) {
		t.Fatalf("the URL answered %d, %d bytes", code, len(body))
	}
	if len(m.nas.Ranges()) == 0 {
		t.Fatal("the NAS was not read")
	}
}

// Where the NAS cannot be read, the matched film streams the computer's video in its place, rather than saying it cannot play.
func TestAMatchedFilmStreamsFromTheComputerWhenTheNASIsOff(t *testing.T) {
	m := startMerged(t, func(dir string, src store.Source, desktop string) { keepHoliday(t, dir, src, desktop, false) })
	m.nas.Close()
	m.app.Press(m.row)
	m.app.WaitFor("id:player-video", wait)
	if link := m.playerSource(); strings.HasSuffix(link, "/Holiday.mp4") {
		// What the system's player asks first, which headless nothing asks.
		if code, _ := fetchRange(t, link, 0, 9); code != http.StatusBadGateway {
			t.Fatalf("the NAS switched off answered %d", code)
		}
	}
	m.app.WaitUntil(wait, func() bool { return strings.HasSuffix(m.playerSource(), "/video.mp4") })
	if code, body := fetch(t, m.playerSource()); code != http.StatusOK || body != string(talkVideo) {
		t.Fatalf("the computer's video answered %d, %d bytes", code, len(body))
	}
	if m.app.Exists("id:player-failed") {
		t.Fatalf("the player says %q", m.texts("id:player-failed"))
	}
}

// The matched film is left at one place, whichever video plays: the NAS's stream, kept on the film of the NAS and sent to the computer as its film's, goes on from there as the computer's video.
func TestAMatchedFilmKeepsOnePlace(t *testing.T) {
	m := startMerged(t, func(dir string, src store.Source, desktop string) { keepHoliday(t, dir, src, desktop, false) })
	m.app.Press(m.row)
	m.app.WaitFor("id:player-video", wait)
	if !strings.HasSuffix(m.playerSource(), "/Holiday.mp4") {
		t.Fatalf("the player plays %q", m.playerSource())
	}
	m.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	m.at(6 * sec)
	m.app.Press("id:player-minimize")
	m.app.WaitUntil(wait, func() bool { f, _ := m.kept(m.nasFilm()); return f.PositionMS == 6000 })
	m.app.WaitUntil(wait, func() bool { return strings.Contains(m.texts(m.row), msgs.FilmsWatchedTo("0:06")) })

	m.nas.Close()
	m.app.Press(m.row)
	if link := m.playerSource(); strings.HasSuffix(link, "/Holiday.mp4") {
		fetchRange(t, link, 0, 9)
	}
	m.app.WaitUntil(wait, func() bool { return strings.HasSuffix(m.playerSource(), "/video.mp4") })
	if got := m.texts("id:player-at"); got != "0:06" {
		t.Fatalf("the computer's video starts at %q", got)
	}
}

// The place goes to the computer as its film's, so the computer and the other devices go on from it too.
func TestAMatchedFilmsPlaceGoesToTheComputer(t *testing.T) {
	dir := t.TempDir()
	src := davSource(davFilms(t, false))
	d := holidayDesktop(t, holidayKey(t, src, "127.0.0.1"))
	desktop := store.DesktopID(d.Fingerprint)
	keepHoliday(t, dir, src, desktop, true)
	p := startWithSourceAndDesktop(t, dir, src, d)
	p.app.WaitFor(filmRow(desktop, "f2"), wait)
	p.app.Press(filmRow(desktop, "f2"))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(6 * sec)
	p.app.Press("id:player-minimize")
	p.app.WaitUntil(wait, func() bool {
		r, ok := d.State("lib", sync.KindPosition, "f2")
		return ok && string(r.Fields[sync.FieldPositionMS].Value) == "6000"
	})
}

// A computer's film downloaded to the phone and left later than the film of the NAS matched to it hands that place to the film of the NAS, which keeps the place of both from then on.
func TestAMatchedFilmTakesTheLaterPlace(t *testing.T) {
	m := startMerged(t, func(dir string, src store.Source, desktop string) {
		n := keepHoliday(t, dir, src, desktop, false)
		n.PositionMS, n.PositionAt = 2000, now()
		c := keepComputerHoliday(t, dir, desktop)
		c.PositionMS, c.PositionAt = 7000, now().Add(sec)
		for _, f := range []store.Film{n, c} {
			if err := store.Open(dir).SaveFilm(f); err != nil {
				t.Fatal(err)
			}
		}
	})
	m.app.WaitUntil(wait, func() bool { f, _ := m.kept(m.nasFilm()); return f.PositionMS == 7000 })
	m.app.WaitUntil(wait, func() bool { return strings.Contains(m.texts(m.row), msgs.FilmsWatchedTo("0:07")) })
}

// Saved for offline from its options, the matched film downloads from the NAS while the NAS answers, and from the computer otherwise; either way the row says it is downloaded.
func TestAMatchedFilmDownloadsFromTheNASOrElseTheComputer(t *testing.T) {
	for name, off := range map[string]bool{"from the NAS": false, "from the computer": true} {
		t.Run(name, func(t *testing.T) {
			m := startMerged(t, func(dir string, src store.Source, desktop string) { keepHoliday(t, dir, src, desktop, false) })
			if off {
				m.nas.Close()
			}
			options := "id:films-options-" + m.desktop + "-f2"
			m.app.Press(options)
			m.choose(msgs.FilmsSaveOffline())
			nasVideo := filepath.Join("films", store.SourceOwner(m.src.ID), store.SourceFilmID("Holiday.mp4"), "video.mp4")
			computerVideo := filepath.Join("films", m.desktop, "f2", "video.mp4")
			got, other := nasVideo, computerVideo
			if off {
				got, other = computerVideo, nasVideo
			}
			m.waitFile(got, int64(len(talkVideo)))
			if _, err := os.Stat(filepath.Join(m.dir, other)); err == nil {
				t.Fatalf("%s was downloaded too", other)
			}
			m.app.WaitUntil(wait, func() bool { return strings.Contains(m.texts(m.row), msgs.FilmDownloaded()) })
		})
	}
}

// Deleted from the phone, the matched film takes both copies with it and the computer's lines it took, and the computer's film is listed as before, not on the phone.
func TestDeletingAMatchedFilmDeletesBothCopies(t *testing.T) {
	m := startMerged(t, func(dir string, src store.Source, desktop string) {
		keepHoliday(t, dir, src, desktop, true)
		keepComputerHoliday(t, dir, desktop)
	})
	m.app.Press("id:films-options-" + m.desktop + "-f2")
	m.choose(msgs.FilmDelete())
	m.app.Answer(1)
	m.waitFile(filepath.Join("films", store.SourceOwner(m.src.ID), store.SourceFilmID("Holiday.mp4")), -1)
	m.waitFile(filepath.Join("films", m.desktop, "f2"), -1)
	m.app.WaitUntil(wait, func() bool { return !strings.Contains(m.texts(m.row), msgs.FilmDownloaded()) })
	if got := m.texts("id:films-count"); got != msgs.FilmsCount(1) {
		t.Fatalf("the count says %q", got)
	}
	if list, _ := store.Open(m.dir).Sources(); len(list) != 1 {
		t.Fatalf("the source went with the film: %+v", list)
	}
}

// With its computer off, the matched film is listed once as the phone last took it from the computer, a search finds its lines, and it plays them over the video downloaded from the NAS.
func TestAMatchedFilmShowsTheComputersLinesWithTheComputerOff(t *testing.T) {
	dir := t.TempDir()
	src := davSource(davFilms(t, false))
	d := holidayDesktop(t, holidayKey(t, src, "127.0.0.1"))
	desktop := store.DesktopID(d.Fingerprint)
	n := keepHoliday(t, dir, src, desktop, true)
	onboarded(t, dir)
	if err := store.Open(dir).SaveSources([]store.Source{src}); err != nil {
		t.Fatal(err)
	}
	secrets := pairedTokens(t, dir, d)
	secrets[store.SourceSecretAccount(src.ID, nas.Password)] = davtest.Password
	d.SetOff(true)
	p := startPhone(t, dir, nsgotest.Secrets(secrets))
	row := filmRow(desktop, "f2")
	p.app.WaitFor(row, wait)
	if p.app.Exists(filmRow(n.Desktop, n.Film.ID)) {
		t.Fatal("the film of the NAS is listed beside the computer's")
	}
	if got := p.texts(row); !strings.Contains(got, "Holiday on the NAS") {
		t.Fatalf("the film's row reads %q", got)
	}
	p.app.Type("id:films-search", "HOW ARE")
	hit := "id:films-hit-" + desktop + "-f2"
	p.app.WaitFor(hit, wait)
	if got, want := p.texts(hit), msgs.FilmsHit("0:03", "How are you?"); got != want {
		t.Fatalf("the hit says %q, want %q", got, want)
	}
	p.app.Clear("id:films-search")
	p.app.WaitFor(row, wait)
	p.app.Press(row)
	p.app.WaitFor("id:player-video", wait)
	if want := filepath.Join(dir, "films", n.Desktop, n.Film.ID, "video.mp4"); p.playerSource() != want {
		t.Fatalf("the player plays %q", p.playerSource())
	}
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") })
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(3500 * sec / 1000)
	if got := p.texts("id:player-card-translation"); got != "你好吗？" {
		t.Fatalf("the card's translation is %q", got)
	}
}
