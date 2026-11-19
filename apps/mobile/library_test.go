package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// newDesktopWithMovies is newDesktop with a second library, Movies, of a film from Bilibili an hour older than the talk.
func newDesktopWithMovies(t *testing.T) *synctest.Desktop {
	t.Helper()
	d := newDesktop(t)
	d.AddLibrary("movies", "Movies")
	d.AddFilm(synctest.Film{Library: "movies", Video: []byte("v"),
		Film: sync.Film{ID: "f2", Title: "Evening Talk", Source: "bilibili", DurationMS: 65_000, Spoken: "ja", Updated: now().Add(-time.Hour)}})
	return d
}

// alertLabels are the buttons of the alert shown last.
func (p *phone) alertLabels() []string {
	p.t.Helper()
	alerts := p.app.Alerts()
	if len(alerts) == 0 {
		p.t.Fatal("no alert shows")
	}
	var out []string
	for _, a := range alerts[len(alerts)-1].Actions {
		out = append(out, a.Label)
	}
	return out
}

// The Library lists the films of every library of the paired computer with those the phone keeps from another, the latest first, each with its source and length; pull to refresh reads them again.
func TestTheLibraryMergesTheComputersFilms(t *testing.T) {
	d := newDesktopWithMovies(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	keepTalk(t, dir)
	p := startPhone(t, dir, secrets)
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	if got := p.texts(talkRow(id)); !strings.Contains(got, "Morning Talk") || !strings.Contains(got, "YouTube · 0:10") {
		t.Fatalf("the talk's row shows %q", got)
	}
	if got := p.texts(filmRow(id, "f2")); !strings.Contains(got, "Bilibili · 1:05") {
		t.Fatalf("the Movies film's row shows %q", got)
	}
	if got := p.texts(talkRow("gonedesk")); !strings.Contains(got, "Other Sites · 0:10 · "+msgs.FilmDownloaded()) {
		t.Fatalf("the film kept from a computer gone shows %q", got)
	}
	if got := p.texts("id:films-count"); got != msgs.FilmsCount(3) {
		t.Fatalf("the list counts %q", got)
	}
	d.AddFilm(synctest.Film{Library: "lib", Video: []byte("v"), Film: sync.Film{ID: "f3", Title: "Night Talk", Spoken: "ja", Updated: now()}})
	p.app.PullToRefresh("id:films-scroll")
	p.app.WaitFor(filmRow(id, "f3"), wait)
}

// The chips narrow the list to a source, and the switcher to a library.
func TestChipsAndTheSwitcherNarrowTheList(t *testing.T) {
	d := newDesktopWithMovies(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	p.app.Press("id:films-source-bilibili")
	p.app.WaitForGone(talkRow(id), wait)
	p.app.Find(filmRow(id, "f2"))
	p.app.Press("id:films-source-local")
	p.app.WaitFor("id:films-empty", wait)
	if got := p.texts("id:films-empty"); got != msgs.FilmsNoSource() {
		t.Fatalf("a source with no films says %q", got)
	}
	p.app.Press("id:films-source-all")
	p.app.WaitFor(talkRow(id), wait)

	p.app.Press("id:films-library")
	want := []string{msgs.FilmsAllLibraries(), msgs.FilmsLibraryOn("Japanese Class", "Studio"), msgs.FilmsLibraryOn("Movies", "Studio"), msgs.CommonCancel()}
	if got := p.alertLabels(); !slices.Equal(got, want) {
		t.Fatalf("the switcher offers %q, want %q", got, want)
	}
	p.app.Answer(2)
	p.app.WaitForGone(talkRow(id), wait)
	if got := p.texts("id:films-library"); got != msgs.FilmsLibraryOn("Movies", "Studio") {
		t.Fatalf("the switcher says %q", got)
	}
	p.app.Press("id:films-library")
	p.app.Answer(0)
	p.app.WaitFor(talkRow(id), wait)
}

// A film left partway on the computer comes first to go on with, saying where; on the phone it plays from there, and one not on the phone opens its page to download it.
func TestContinueWatchingWhereTheComputerLeftOff(t *testing.T) {
	d := newDesktop(t)
	d.SetState("lib", now().Add(time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(4000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	p := startPhone(t, dir, secrets)
	card := "id:films-continue-" + id + "-f1"
	p.app.WaitFor(card, wait)
	if got, want := p.texts("id:films-continue-meta-"+id+"-f1"), msgs.FilmsLeftOn("Studio", "0:04"); got != want {
		t.Fatalf("the card says %q, want %q", got, want)
	}
	if got := p.texts(card); !strings.Contains(got, msgs.FilmsLeftHere("Studio")) {
		t.Fatalf("the card's poster says %q, without the computer it was left on", got)
	}
	p.app.Press(card)
	p.app.WaitFor("id:film-download", wait)

	keepTalkFrom(t, dir, id)
	p = startPhone(t, dir, secrets)
	p.app.WaitFor(card, wait)
	p.app.Press(card)
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 4000 })
}

// A search finds the lines of the films on the phone, saying the line it found; pressed, the film plays from it. A search that finds nothing says so.
func TestSearchFindsTheLinesOnThePhone(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.Type("id:films-search", "tide")
	p.app.WaitFor("id:films-empty", wait)
	if got, want := p.texts("id:films-empty"), msgs.FilmsNoResults("tide"); got != want {
		t.Fatalf("a search that finds nothing says %q, want %q", got, want)
	}
	if p.app.Exists("id:films-continue") {
		t.Fatal("the films to go on with show over the search's results")
	}
	p.app.Clear("id:films-search")
	p.app.Type("id:films-search", "HOW ARE")
	hit := "id:films-hit-gonedesk-f1"
	p.app.WaitFor(hit, wait)
	if got, want := p.texts(hit), msgs.FilmsHit("0:03", "How are you?"); got != want {
		t.Fatalf("the hit says %q, want %q", got, want)
	}
	if got := p.texts("id:films-count"); got != msgs.FilmsCount(1) {
		t.Fatalf("the results count %q", got)
	}
	p.app.Press(talkRow("gonedesk"))
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 3000 })
}

// A film's options download it and, once it is on the phone, play it or delete it from the phone after asking.
func TestAFilmsOptionsDownloadAndDeleteIt(t *testing.T) {
	d := newDesktop(t)
	// The computer's own actions on a film are films_test.go's; here the options are the phone's.
	d.SetNoFilmOps(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	options := "id:films-options-" + id + "-f1"
	p.app.WaitFor(options, wait)
	p.app.Press(options)
	if got, want := p.alertLabels(), []string{msgs.FilmsDownloadSize("977 KB"), msgs.FilmsDetails(), msgs.CommonCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film not on the phone are %q, want %q", got, want)
	}
	if !p.app.Alerts()[0].Sheet {
		t.Fatal("a phone shows a film's options in something other than an action sheet")
	}
	p.app.Answer(0)
	folder := filepath.Join("films", id, "f1")
	p.waitFile(filepath.Join(folder, "video.mp4"), int64(len(talkVideo)))
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloaded(), wait)
	p.app.Press(options)
	if got, want := p.alertLabels(), []string{msgs.FilmPlay(), msgs.FilmDelete(), msgs.FilmsDetails(), msgs.CommonCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film on the phone are %q, want %q", got, want)
	}
	p.app.Answer(1)
	if got := p.app.Alerts(); len(got) != 1 || got[0].Title != msgs.FilmDeleteTitle("Morning Talk") {
		t.Fatalf("deleting from the options asks %+v", got)
	}
	p.app.Answer(1)
	p.waitFile(folder, -1)
}

// A desktop that does not answer says so, and the films downloaded from it still show and open.
func TestOfflineTheDownloadsStillShow(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.pair(d)
	p.app.WaitUntil(wait, func() bool { kept, _ := store.Open(p.dir).Desktops(); return len(kept) == 1 })
	d.Close()
	dir := p.dir
	keepTalk(t, dir)
	p = startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	problem := "id:computers-problem-" + store.DesktopID(d.Fingerprint)
	p.app.WaitFor(problem, wait)
	if got, want := p.texts(problem), msgs.ComputersUnreachable("Studio"); !strings.Contains(got, want) {
		t.Fatalf("the desktop's card says %q, want %q", got, want)
	}
	// A computer that does not answer offers to try again, not to pair again.
	id := store.DesktopID(d.Fingerprint)
	p.app.Find("id:computers-retry-" + id)
	if p.app.Exists("id:computers-pair-again-" + id) {
		t.Fatal("an unreachable desktop offers to pair again")
	}
	p.app.Press(talkRow("gonedesk"))
	p.app.WaitFor("id:film-play", wait)
}

// A desktop that revoked the phone asks to pair again, and its films go from the list.
func TestARevokedPhoneIsAskedToPairAgain(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.pair(d)
	id := store.DesktopID(d.Fingerprint)
	// The learning state is read once pairing is done; revoked before then, the phone would show the page that says so over the Library (learn_test.go).
	p.waitLearned(id)
	d.RevokeAll()
	p.app.PullToRefresh("id:films-scroll")
	p.app.WaitFor("id:computers-pair-again-"+id, wait)
	if got, want := p.texts("id:computers-problem-"+id), msgs.ComputersUnpaired("Studio"); !strings.Contains(got, want) {
		t.Fatalf("the desktop's card says %q, want %q", got, want)
	}
	if p.app.Exists(talkRow(id)) || p.app.Exists("id:computers-retry-"+id) {
		t.Fatal("beside the request to pair again the Library shows the desktop's films or a retry")
	}
}

// A search reads the lines kept on the phone once: typing on, which leaves the films on the phone as they are, does not read them again.
func TestSearchReadsTheKeptLinesOnceAsItIsTyped(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.Type("id:films-search", "how")
	hit := "id:films-hit-gonedesk-f1"
	p.app.WaitFor(hit, wait)
	// The lines on disk go from under the search: read again, the hit would go with them.
	if err := store.Open(dir).SaveCues(f.Desktop, f.Film.ID, nil); err != nil {
		t.Fatal(err)
	}
	p.app.Type("id:films-search", " are")
	for range 30 {
		p.app.Settle()
		if !p.app.Exists(hit) {
			t.Fatal("typing on read the lines kept on the phone again")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A computer out of reach on a pull to refresh leaves the library chosen as it was, the switcher there to take the list back to every library.
func TestAFailedRefreshKeepsTheLibraryChosen(t *testing.T) {
	d := newDesktopWithMovies(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepTalkFrom(t, dir, id)
	p := startPhone(t, dir, secrets)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	p.app.Press("id:films-library")
	p.app.Answer(2)
	p.app.WaitForGone(talkRow(id), wait)
	d.Close()
	p.app.PullToRefresh("id:films-scroll")
	p.app.WaitFor("id:computers-retry-"+id, wait)
	if got, want := p.texts("id:films-library"), msgs.FilmsLibraryOn("Movies", "Studio"); got != want {
		t.Fatalf("after a failed refresh the switcher says %q, want %q", got, want)
	}
	if p.app.Exists(talkRow(id)) {
		t.Fatal("after a failed refresh the list shows the films of every library")
	}
	p.app.Press("id:films-library")
	p.app.Answer(0)
	p.app.WaitFor(talkRow(id), wait)
}

// A film's options pause a download under way, resume it and cancel it, which deletes what it downloaded.
func TestAFilmsOptionsPauseResumeAndCancelItsDownload(t *testing.T) {
	d := newDesktop(t)
	// The computer's own actions on a film are films_test.go's; here the options are the phone's.
	d.SetNoFilmOps(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	options := "id:films-options-" + id + "-f1"
	p.app.WaitFor(options, wait)
	d.Stall(300_000)
	p.app.Press(options)
	p.app.Answer(0)
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloading(30), wait)
	p.app.Press(options)
	if got, want := p.alertLabels(), []string{msgs.FilmPause(), msgs.FilmCancel(), msgs.FilmsDetails(), msgs.CommonCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film downloading are %q, want %q", got, want)
	}
	p.app.Answer(0)
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmPaused(30), wait)
	folder := filepath.Join("films", id, "f1")
	p.waitFile(filepath.Join(folder, store.PartFile), 300_000)
	p.app.Press(options)
	if got, want := p.alertLabels(), []string{msgs.FilmResume(), msgs.FilmCancel(), msgs.FilmsDetails(), msgs.CommonCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film paused are %q, want %q", got, want)
	}
	p.app.Answer(0)
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloading(60), wait)
	p.app.Press(options)
	p.app.Answer(1)
	p.waitFile(folder, -1)
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10", wait)
}

// Played from the beginning from its options, a film the computer left partway is the phone's from then on: it goes from the films to go on with, and its row no longer says where the computer left it.
func TestPlayFromTheBeginningOutdatesTheComputersPosition(t *testing.T) {
	d := newDesktop(t)
	// The computer's own actions on a film are films_test.go's; here the options are the phone's.
	d.SetNoFilmOps(true)
	d.SetState("lib", now().Add(-time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(4000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepTalkFrom(t, dir, id)
	p := startPhone(t, dir, secrets)
	card := "id:films-continue-" + id + "-f1"
	p.app.WaitFor(card, wait)
	p.app.Press("id:films-options-" + id + "-f1")
	if got, want := p.alertLabels(), []string{msgs.FilmPlay(), msgs.FilmsPlayFromStart(), msgs.FilmDelete(), msgs.FilmsDetails(), msgs.CommonCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film left partway are %q, want %q", got, want)
	}
	p.app.Answer(1)
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool {
		films, err := store.Open(dir).Films()
		return err == nil && len(films) == 1 && films[0].PositionMS == 0 && !films[0].PositionAt.IsZero()
	})
	p = startPhone(t, dir, secrets)
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloaded(), wait)
	if p.app.Exists(card) {
		t.Fatal("a film played from the beginning still shows where the computer left it")
	}
}

// A tablet shows a film's options in a dialog in the middle of the screen, not an action sheet.
func TestATabletShowsAFilmsOptionsInADialog(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the idiom is the platform's on a target")
	}
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	options := "id:films-options-" + store.DesktopID(d.Fingerprint) + "-f1"
	p.app.WaitFor(options, wait)
	p.app.Press(options)
	if alerts := p.app.Alerts(); len(alerts) == 0 || alerts[len(alerts)-1].Sheet {
		t.Fatal("a tablet shows a film's options as an action sheet")
	}
}

// A film a job on the computer added shows in the Library once the phone hears the job finish, without pulling to refresh.
func TestTheLibraryShowsTheFilmAJobAdded(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(talkRow(id), wait)
	// The phone has to have seen the job running: one it first hears of already done is an old one, not one that just finished.
	moved(d, "job1", "https://youtu.be/night000001", sync.JobRunning, 0, 0.5)
	p.app.WaitFor("id:films-jobs", wait)
	p.app.Press("id:films-jobs")
	p.app.WaitFor("id:jobs-job-job1", wait)
	p.app.HeaderBack("id:jobs-page")
	p.app.WaitForGone("id:jobs-page", wait)
	d.AddFilm(synctest.Film{Library: "lib", Video: []byte("v"), Film: sync.Film{ID: "f3", Title: "Night Talk", Spoken: "ja", Updated: now()}})
	done := moved(d, "job1", "https://youtu.be/night000001", sync.JobDone, 3, 0)
	done.Film = "f3"
	d.SetJob(done)
	p.app.WaitFor(filmRow(id, "f3"), wait)
}
