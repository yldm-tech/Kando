package main

import (
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
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

// sheetLabels are the titles of the rows of the sheet whose rows are id'd prefix0, prefix1, …, once it shows.
func (p *phone) sheetLabels(prefix string) []string {
	p.t.Helper()
	p.app.WaitFor("id:"+prefix+"0", wait)
	var out []string
	for i := 0; p.app.Exists("id:" + prefix + strconv.Itoa(i)); i++ {
		out = append(out, strings.Split(p.texts("id:"+prefix+strconv.Itoa(i)), "\n")[0])
	}
	return out
}

// optionLabels are the options of the film whose options sheet shows.
func (p *phone) optionLabels() []string { p.t.Helper(); return p.sheetLabels("films-option-") }

// choose presses the option label of the film's options sheet, which closes it.
func (p *phone) choose(label string) {
	p.t.Helper()
	labels := p.optionLabels()
	i := slices.Index(labels, label)
	if i < 0 {
		p.t.Fatalf("the options are %q, not %q", labels, label)
	}
	p.app.Press("id:films-option-" + strconv.Itoa(i))
	p.app.WaitForGone("id:films-options-sheet", wait)
}

// libraryLabels are the libraries the library sheet offers.
func (p *phone) libraryLabels() []string { p.t.Helper(); return p.sheetLabels("films-library-option-") }

// pickLibrary presses the library sheet's row i, which closes it.
func (p *phone) pickLibrary(i int) {
	p.t.Helper()
	p.app.WaitFor("id:films-library-option-"+strconv.Itoa(i), wait)
	p.app.Press("id:films-library-option-" + strconv.Itoa(i))
	p.app.WaitForGone("id:films-library-sheet", wait)
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
	if got := p.texts(talkRow("gonedesk")); !strings.Contains(got, msgs.FilmsSourceWeb()+" · 0:10 · "+msgs.FilmDownloaded()) {
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
	want := []string{msgs.FilmsAllLibraries(), "Japanese Class", "Movies"}
	if got := p.libraryLabels(); !slices.Equal(got, want) {
		t.Fatalf("the switcher offers %q, want %q", got, want)
	}
	p.pickLibrary(2)
	p.app.WaitForGone(talkRow(id), wait)
	if got := p.texts("id:films-library"); got != "Movies" {
		t.Fatalf("the switcher says %q", got)
	}
	if got := p.app.Find("id:films-library").Props.Label; got != msgs.FilmsLibrary(msgs.FilmsLibraryOn("Movies", "Studio")) {
		t.Fatalf("the switcher is named %q", got)
	}
	p.app.Press("id:films-library")
	p.pickLibrary(0)
	p.app.WaitFor(talkRow(id), wait)
}

// A film left partway on the computer comes first to go on with, saying where; on the phone it plays from there, and one not on the phone opens its page to download it.
func TestContinueWatchingWhereTheComputerLeftOff(t *testing.T) {
	d := newDesktop(t)
	d.SetState("lib", now().Add(time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(6000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	p := startPhone(t, dir, secrets)
	card := "id:films-continue-" + id + "-f1"
	p.app.WaitFor(card, wait)
	if got, want := p.texts("id:films-continue-meta-"+id+"-f1"), msgs.FilmsLeftOn("Studio", "0:06"); got != want {
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
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 6000 })
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

// A search of spaces alone, as one left after clearing the field, searches for nothing: the Library stays as it was, headed as every film. Spaces around a search are not part of it.
func TestASearchOfSpacesSearchesForNothing(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.Type("id:films-search", "  ")
	if got := p.texts("id:films-list"); !strings.HasPrefix(got, msgs.FilmsAll()+"\n") {
		t.Fatalf("under a search of spaces the list reads %q, want it headed %q", got, msgs.FilmsAll())
	}
	if p.app.Exists("id:films-empty") || !p.app.Exists(talkRow("gonedesk")) {
		t.Fatal("a search of spaces narrows the list")
	}
	p.app.Type("id:films-search", "tide ")
	p.app.WaitFor("id:films-empty", wait)
	if got, want := p.texts("id:films-empty"), msgs.FilmsNoResults("tide"); got != want {
		t.Fatalf("a search with spaces around it says %q, want %q", got, want)
	}
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
	if got, want := p.optionLabels(), []string{msgs.FilmPlay(), msgs.FilmsSaveOffline()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film not on the phone are %q, want %q", got, want)
	}
	if got, want := p.texts("id:films-option-1"), msgs.FilmsSaveOffline()+"\n"+msgs.FilmsAboutSize("977 KB"); got != want {
		t.Fatalf("the download option reads %q, want %q", got, want)
	}
	p.choose(msgs.FilmsSaveOffline())
	folder := filepath.Join("films", id, "f1")
	p.waitFile(filepath.Join(folder, "video.mp4"), int64(len(talkVideo)))
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloaded(), wait)
	p.app.Press(options)
	if got, want := p.optionLabels(), []string{msgs.FilmPlay(), msgs.FilmDelete()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film on the phone are %q, want %q", got, want)
	}
	p.choose(msgs.FilmDelete())
	p.app.WaitUntil(wait, func() bool { return len(p.app.Alerts()) == 1 })
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
	p.app.WaitFor("id:player-video", wait)
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
	p.pickLibrary(2)
	p.app.WaitForGone(talkRow(id), wait)
	d.Close()
	p.app.PullToRefresh("id:films-scroll")
	p.app.WaitFor("id:computers-retry-"+id, wait)
	if got, want := p.texts("id:films-library"), "Movies"; got != want {
		t.Fatalf("after a failed refresh the switcher says %q, want %q", got, want)
	}
	if p.app.Exists(talkRow(id)) {
		t.Fatal("after a failed refresh the list shows the films of every library")
	}
	p.app.Press("id:films-library")
	p.pickLibrary(0)
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
	p.choose(msgs.FilmsSaveOffline())
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloading(30), wait)
	p.app.Press(options)
	if got, want := p.optionLabels(), []string{msgs.FilmPause(), msgs.FilmCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film downloading are %q, want %q", got, want)
	}
	p.choose(msgs.FilmPause())
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmPaused(30), wait)
	folder := filepath.Join("films", id, "f1")
	p.waitFile(filepath.Join(folder, store.PartFile), 300_000)
	p.app.Press(options)
	if got, want := p.optionLabels(), []string{msgs.FilmResume(), msgs.FilmCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film paused are %q, want %q", got, want)
	}
	p.choose(msgs.FilmResume())
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloading(60), wait)
	p.app.Press(options)
	p.choose(msgs.FilmCancel())
	p.waitFile(folder, -1)
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10", wait)
}

// Played from the beginning from its options, a film the computer left partway is the phone's from then on: it goes from the films to go on with, and its row no longer says where the computer left it.
func TestPlayFromTheBeginningOutdatesTheComputersPosition(t *testing.T) {
	d := newDesktop(t)
	// The computer's own actions on a film are films_test.go's; here the options are the phone's.
	d.SetNoFilmOps(true)
	d.SetState("lib", now().Add(-time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(6000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepTalkFrom(t, dir, id)
	p := startPhone(t, dir, secrets)
	card := "id:films-continue-" + id + "-f1"
	p.app.WaitFor(card, wait)
	p.app.Press("id:films-options-" + id + "-f1")
	if got, want := p.optionLabels(), []string{msgs.FilmPlay(), msgs.FilmsPlayFromStart(), msgs.FilmDelete()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film left partway are %q, want %q", got, want)
	}
	p.choose(msgs.FilmsPlayFromStart())
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

// A tablet shows a film's options in a dialog in the middle of the screen, not a sheet from the bottom.
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
	p.app.WaitFor("id:films-option-0", wait)
	if f := p.app.Find("id:films-options-sheet").Bounds(); f.Size.Width > 600 || f.Origin.Y+f.Size.Height > 800 {
		t.Fatalf("a tablet shows a film's options at %v, not in the middle of the screen", f)
	}
}

// A tablet upright has no sidebar, so its one-row header starts with the library switcher, beside the search and the Add pill; the films lie in a grid of cards, each with its options under its poster, and the jobs under way are the stats' fourth tile.
func TestAnUprightTabletHeadsTheLibraryWithTheSwitcher(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the idiom is the platform's on a target")
	}
	d := newDesktopWithMovies(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(820, 1180), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet, OS: core.OSAndroid}))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(talkRow(id), wait)
	if p.app.Exists("id:sidebar") {
		t.Fatal("an upright tablet shows the sidebar")
	}
	// The header is one row: the switcher, the title, the search and the Add pill.
	sw, search, add := p.app.Find("id:films-library").Bounds(), p.app.Find("id:films-search").Bounds(), p.app.Find("id:films-add").Bounds()
	mid := func(b core.Bounds) core.Points { return b.Origin.Y + b.Size.Height/2 }
	if d := mid(sw) - mid(search); d > 4 || d < -4 || mid(add)-mid(search) > 4 || mid(add)-mid(search) < -4 {
		t.Fatalf("the switcher %v, the search %v and Add %v are not one row", sw, search, add)
	}
	if !(sw.Origin.X < search.Origin.X && search.Origin.X < add.Origin.X) {
		t.Fatalf("the header runs switcher %v, search %v, Add %v, want them in that order", sw, search, add)
	}
	p.app.Press("id:films-library")
	if got, want := p.libraryLabels(), []string{msgs.FilmsAllLibraries(), "Japanese Class", "Movies"}; !slices.Equal(got, want) {
		t.Fatalf("the switcher offers %q, want %q", got, want)
	}
	p.pickLibrary(0)

	// The grid: three cards a row, each one's options just under its poster, at the card's end.
	rows := p.app.Find("id:films-rows").Bounds()
	talk, evening := p.app.Find(talkRow(id)).Bounds(), p.app.Find(filmRow(id, "f2")).Bounds()
	if talk.Size.Width > rows.Size.Width/3 || evening.Size.Width > rows.Size.Width/3 {
		t.Fatalf("the cards are %v and %v wide in a grid %v, want a third of it", talk, evening, rows)
	}
	poster := talk.Size.Width * 9 / 16
	if opts := p.app.Find("id:films-options-" + id + "-f1").Bounds(); opts.Origin.Y < talk.Origin.Y+poster-4 || opts.Origin.Y > talk.Origin.Y+poster+2 || opts.Origin.X+opts.Size.Width < talk.Origin.X+talk.Size.Width {
		t.Fatalf("a card's options lie at %v, want under its poster at the end of the card %v", opts, talk)
	}

	// A job under way shows as the stats' fourth tile, not a card under them.
	moved(d, "job1", "https://youtu.be/night000001", sync.JobRunning, 0, 0.5)
	p.app.WaitFor("id:films-stats >> id:films-jobs-card", wait)
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
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-job-job1", wait)
	p.openTab(filmsTab, msgs.FilmsTitle())
	d.AddFilm(synctest.Film{Library: "lib", Video: []byte("v"), Film: sync.Film{ID: "f3", Title: "Night Talk", Spoken: "ja", Updated: now()}})
	done := moved(d, "job1", "https://youtu.be/night000001", sync.JobDone, 3, 0)
	done.Film = "f3"
	d.SetJob(done)
	p.app.WaitFor(filmRow(id, "f3"), wait)
}

// While a computer has jobs under way the Library shows a card under the search: which computer processes how many videos, the one running now and its step, and how many need the user; pressed, it opens Jobs. It stays while searching.
func TestTheJobsCardSaysWhatTheComputerIsDoing(t *testing.T) {
	d := newDesktop(t)
	running := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 2, 0.84)
	running.Title = "Morning Market in Kyoto"
	d.SetJob(running)
	moved(d, "job2", "https://youtu.be/ScMzIvxBSi4", sync.JobFailed, 0, 0)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.app.WaitFor("id:films-jobs-card-needs", wait)
	if got, want := p.texts("id:films-jobs-card-title"), msgs.FilmsJobsCardTitle("Studio", 1); got != want {
		t.Fatalf("the jobs card's title is %q, want %q", got, want)
	}
	if got, want := p.texts("id:films-jobs-card-line"), "Morning Market in Kyoto · "+msgs.JobsStepPercent(msgs.JobsStepTranslate(), 84); got != want {
		t.Fatalf("the jobs card's line is %q, want %q", got, want)
	}
	if got, want := p.texts("id:films-jobs-card-needs"), msgs.FilmsJobsCardNeedsYou(1); got != want {
		t.Fatalf("the jobs card's pill is %q, want %q", got, want)
	}
	p.app.Type("id:films-search", "zzz")
	p.app.WaitFor("id:films-empty", wait)
	if !p.app.Exists("id:films-jobs-card") {
		t.Fatal("a search hides the jobs card")
	}
	p.app.Press("id:films-jobs-card")
	p.app.WaitFor("id:jobs-page", wait)
}

// A storage that no longer takes the computer's key says so on the Library, and the films not on the phone, which come from it away from the computer, fade.
func TestTheLibrarySaysWhenTheStorageFails(t *testing.T) {
	d := newDesktop(t)
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.app.WaitFor(talkRow(id), wait)
	if p.app.Exists("id:films-storage") {
		t.Fatal("the Library warns of a storage that works")
	}
	d.SetOff(true)
	bucket.Refuse(http.StatusForbidden, "SignatureDoesNotMatch")
	p.app.WaitFor("id:films-storage", reconnect)
	if got, want := p.texts("id:films-storage-title"), msgs.FilmsStorageKeysTitle(); got != want {
		t.Fatalf("the storage banner says %q, want %q", got, want)
	}
	if got := p.app.Find(talkRow(id)).Style.Opacity; got > 0.46 || got < 0.44 {
		t.Fatalf("a film not on the phone shows at opacity %v while the storage fails", got)
	}
}

// The read of a computer's films the phone makes as it begins following its events, which may end as the computer goes, keeps the films the Library lists: the computer's reach is the follower's to tell.
func TestTheLibraryKeepsItsFilmsWhenTheComputerGoesDuringACatchUp(t *testing.T) {
	d := newDesktop(t)
	jobs := d.HoldJobs()
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(talkRow(id), wait)
	libraries := d.Hold()
	defer libraries()
	jobs()
	p.app.WaitUntil(wait, func() bool { return d.HeldLibraries() == 1 })
	d.SetOff(true)
	p.app.WaitUntil(wait, func() bool { return d.HeldLibraries() == 0 })
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		p.app.Settle()
		if !p.app.Exists(talkRow(id)) {
			t.Fatal("the Library drops its films as the computer goes during the phone's read of them")
		}
	}
}

// Before a computer is paired the Library's Add leads to pairing, which a link needs first; the switcher shows only while the phone keeps films, offering their libraries (design 08).
func TestTheLibrarysAddPairsFirst(t *testing.T) {
	empty := startPhone(t, t.TempDir())
	empty.app.WaitFor("id:onboarding-skip", wait)
	empty.app.Press("id:onboarding-skip")
	empty.app.WaitFor("id:films-unpaired", wait)
	if empty.app.Exists("id:films-library") {
		t.Fatal("the switcher shows with no computer paired and no film on the phone")
	}

	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:films-unpaired", wait)
	p.app.WaitFor("id:films-library", wait)
	p.app.Press("id:films-library")
	if got, want := p.libraryLabels(), []string{msgs.FilmsAllLibraries(), "Japanese Class"}; !slices.Equal(got, want) {
		t.Fatalf("the switcher offers %q unpaired, want %q", got, want)
	}
	p.pickLibrary(0)
	p.app.Press("id:films-add")
	p.app.WaitFor("id:pair-page", wait)
}
