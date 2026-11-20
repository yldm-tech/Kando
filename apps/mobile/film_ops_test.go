package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// linesFilm is a film the desktop keeps no video of, only its lines: the talk's.
func linesFilm() synctest.Film {
	return synctest.Film{Library: "lib", Cues: talkCues,
		Film: sync.Film{ID: "f3", Title: "Lines Only", Source: "youtube", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", SubtitlesOnly: true, Updated: now()}}
}

// options opens the options of the film behind the button options once its computer's operations are known, as they are once its queue is followed, which runs beside the listing: until want is among them, it closes them and opens them again. It returns their labels, the options left open.
func (p *phone) options(options, want string) []string {
	p.t.Helper()
	p.app.WaitFor(options, wait)
	p.app.WaitUntil(wait, func() bool {
		p.app.Press(options)
		labels := p.optionLabels()
		if slices.Contains(labels, want) {
			return true
		}
		p.app.Key("Escape")
		p.app.WaitForGone("id:films-options-sheet", wait)
		return false
	})
	return p.optionLabels()
}

// answer presses the button label of the alert shown last.
func (p *phone) answer(label string) {
	p.t.Helper()
	labels := p.alertLabels()
	i := slices.Index(labels, label)
	if i < 0 {
		p.t.Fatalf("the alert offers %q, not %q", labels, label)
	}
	p.app.Answer(i)
}

// A film's options take its subtitles from the computer as a file and share it, have the computer translate it again, which the phone follows in its queue, and delete it from the computer's library after asking, after which the Library lists it no more.
func TestAFilmsOptionsActOnItThroughTheComputer(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	options := "id:films-options-" + id + "-f1"
	labels := p.options(options, msgs.FilmsExportSubtitles())
	if want := []string{msgs.FilmsSaveOffline(), msgs.FilmsExportSubtitles(), msgs.FilmsRetranslate(), msgs.FilmsDeleteFromLibrary()}; !slices.Equal(labels, want) {
		t.Fatalf("the options are %q, want %q", labels, want)
	}
	p.choose(msgs.FilmsExportSubtitles())
	p.app.WaitUntil(wait, func() bool { return len(p.app.Shares) == 1 })
	files := p.app.Shares[0].Files
	if len(files) != 1 || filepath.Base(files[0].Path) != "Morning Talk.dual.srt" {
		t.Fatalf("shared %+v", p.app.Shares[0])
	}
	if data, err := os.ReadFile(files[0].Path); err != nil || !strings.Contains(string(data), "Good morning.") || !strings.Contains(string(data), "早上好。") {
		t.Fatalf("the file shared holds %q, %v", data, err)
	}
	if got := d.Exported(); !slices.Equal(got, []string{"f1"}) {
		t.Fatalf("the computer exported %q", got)
	}

	p.app.Press(options)
	p.choose(msgs.FilmsRetranslate())
	p.app.WaitUntil(wait, func() bool { return slices.Equal(d.Retranslated(), []string{"f1"}) })
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.FilmsRetranslating("Morning Talk", "Studio"))
	})

	p.app.Press(options)
	p.choose(msgs.FilmsDeleteFromLibrary())
	p.app.WaitUntil(wait, func() bool { return len(p.app.Alerts()) > 0 })
	if got := p.app.Alerts(); got[len(got)-1].Title != msgs.FilmsDeleteFromLibraryTitle("Morning Talk") || got[len(got)-1].Message != msgs.FilmsDeleteFromLibraryMessage("Studio") {
		t.Fatalf("deleting from the library asks %+v", got[len(got)-1])
	}
	p.answer(msgs.FilmDeleteConfirm())
	p.app.WaitUntil(wait, func() bool { return slices.Equal(d.Deleted(), []string{"f1"}) })
	p.app.WaitForGone(talkRow(id), wait)
}

// A read-only phone may take a film's subtitles as a file, but not change the film on the computer.
func TestAReadOnlyPhoneOnlyExportsAFilm(t *testing.T) {
	d := newDesktop(t)
	d.SetReadOnly(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	labels := p.options("id:films-options-"+store.DesktopID(d.Fingerprint)+"-f1", msgs.FilmsExportSubtitles())
	if want := []string{msgs.FilmsSaveOffline(), msgs.FilmsExportSubtitles()}; !slices.Equal(labels, want) {
		t.Fatalf("a read-only phone's options are %q, want %q", labels, want)
	}
}

// A film the computer keeps no video of is listed as subtitles only. Its lines are kept on the phone without a video and studied in the player, which shows in the picture's place that the computer has no video, with the way to have it download one; while it does, the player says so, and once it has, the phone downloads the video, which then plays.
func TestAFilmWithoutAVideoIsStudiedByItsLines(t *testing.T) {
	d := newDesktop(t)
	d.AddFilm(linesFilm())
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	row := filmRow(id, "f3")
	p.app.WaitFor(row+" >> text:YouTube · 0:10 · "+msgs.FilmSubtitlesOnly(), wait)
	labels := p.options("id:films-options-"+id+"-f3", msgs.FilmVideoOnComputer("Studio"))
	if want := []string{msgs.FilmSaveLines(), msgs.FilmVideoOnComputer("Studio"), msgs.FilmsExportSubtitles(), msgs.FilmsRetranslate(), msgs.FilmsDeleteFromLibrary()}; !slices.Equal(labels, want) {
		t.Fatalf("the options of a film without a video are %q, want %q", labels, want)
	}
	p.choose(msgs.FilmSaveLines())
	p.app.WaitFor(row+" >> text:YouTube · 0:10 · "+msgs.FilmSubtitlesOnly()+" · "+msgs.FilmLinesSaved(), wait)
	folder := filepath.Join("films", id, "f3")
	p.app.WaitUntil(wait, func() bool { _, err := os.Stat(filepath.Join(p.dir, folder, "subtitles.json")); return err == nil })
	if _, err := os.Stat(filepath.Join(p.dir, folder, store.PartFile)); !os.IsNotExist(err) || len(d.Ranges()) != 0 {
		t.Fatalf("a film without a video was downloaded as one: %v, %q", err, d.Ranges())
	}

	p.app.Press(row)
	p.app.WaitFor("id:film-no-video", wait)
	if got := p.texts("id:film-play"); got != msgs.FilmStudyLines() {
		t.Fatalf("the page's button reads %q", got)
	}
	p.app.Press("id:film-play")
	p.app.WaitFor("id:player-no-video", wait)
	if p.app.Exists("id:player-video") {
		t.Fatal("the player shows a video the phone does not have")
	}
	if got := p.texts("id:player-no-video-status"); got != msgs.FilmNoVideo("Studio") {
		t.Fatalf("the player says %q", got)
	}
	p.app.WaitFor("id:player-lines-pane >> text:How are you?", wait)

	p.app.WaitFor("id:player-no-video-ask", wait)
	p.app.Press("id:player-no-video-ask")
	p.app.WaitUntil(wait, func() bool { return slices.Equal(d.VideosAsked(), []string{"f3"}) })
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-no-video-status") == msgs.FilmVideoComing("Studio") })
	if p.app.Exists("id:player-no-video-ask") {
		t.Fatal("the player still asks for the video the computer is downloading")
	}

	// The computer has the video now: its job is done, and it lists the film with it.
	d.ReplaceVideo("f3", talkVideo)
	job := d.Jobs()[0]
	job.State, job.Film = sync.JobDone, "f3"
	for i := range job.Steps {
		if job.Steps[i].State == sync.StepWaiting {
			job.Steps[i].State, job.Steps[i].Progress = sync.StepDone, 1
		}
	}
	d.SetJob(job)
	p.app.WaitFor("id:player-no-video-download", wait)
	if got := p.texts("id:player-no-video-status"); got != msgs.FilmVideoReady("Studio") {
		t.Fatalf("the player says %q once the computer has the video", got)
	}
	p.app.Press("id:player-no-video-download")
	p.waitFile(filepath.Join(folder, "video.mp4"), int64(len(talkVideo)))
	p.app.WaitFor("id:player-video", wait)
}

// The video of a film kept by its lines, cancelled partway, is gone from the phone and the film is kept by its lines again, to be studied as before.
func TestCancellingTheVideoOfAFilmKeptByItsLinesKeepsThem(t *testing.T) {
	d := newDesktop(t)
	d.AddFilm(linesFilm())
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	row := filmRow(id, "f3")
	p.options("id:films-options-"+id+"-f3", msgs.FilmSaveLines())
	p.choose(msgs.FilmSaveLines())
	p.app.WaitFor(row+" >> text:YouTube · 0:10 · "+msgs.FilmSubtitlesOnly()+" · "+msgs.FilmLinesSaved(), wait)

	// The computer has the video now, which the Library reads again.
	d.ReplaceVideo("f3", talkVideo)
	d.Stall(200_000)
	p.app.PullToRefresh("id:films-scroll")
	p.app.Press(row)
	p.app.WaitFor("id:film-no-video-download", wait)
	p.app.Press("id:film-no-video-download")
	p.app.WaitFor("id:film-cancel", wait)
	folder := filepath.Join("films", id, "f3")
	p.waitFile(filepath.Join(folder, store.PartFile), 200_000)
	p.app.Press("id:film-cancel")
	p.app.WaitFor("id:film-no-video-download", wait)
	p.waitFile(filepath.Join(folder, store.PartFile), -1)
	if got := p.texts("id:film-play"); got != msgs.FilmStudyLines() {
		t.Fatalf("once the video was cancelled the page's button reads %q", got)
	}
	if _, err := os.Stat(filepath.Join(p.dir, folder, "subtitles.json")); err != nil {
		t.Fatalf("the lines went with the video: %v", err)
	}
	p.app.Press("id:film-play")
	p.app.WaitFor("id:player-lines-pane >> text:How are you?", wait)
}

// A tablet's sidebar lists the computers' subscriptions under the sections, with the count of new videos waiting on each; pressed, one opens its sheet over whichever section shows, and the Add button opens the Add sheet.
func TestATabletsSidebarListsTheSubscriptions(t *testing.T) {
	d := newDesktop(t)
	d.AddSubscription(earthScience())
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	id := store.DesktopID(d.Fingerprint)
	row := "id:sidebar-subscription-" + id + "-s1"
	p.app.WaitFor(row, wait)
	if got := p.texts("id:sidebar-subscription-new-" + id + "-s1"); got != msgs.FilmsSubscriptionNew(2) {
		t.Fatalf("the sidebar's count reads %q", got)
	}
	if got, want := p.app.Find(row).Props.Label, msgs.FilmsSubscriptionLabel("Earth Science", 2); got != want {
		t.Fatalf("the sidebar's subscription is read as %q, want %q", got, want)
	}
	p.app.Press("id:sidebar-tab-learn")
	p.app.Press(row)
	p.app.WaitFor("id:films-subscription-sheet >> text:How Rivers Shape Landscapes", wait)
	p.app.Press("id:films-subscription-done")
	p.app.WaitForGone("id:films-subscription-sheet", wait)

	p.app.WaitFor("id:sidebar-subscriptions-add", wait)
	p.app.Press("id:sidebar-subscriptions-add")
	p.app.WaitFor("id:add-link", wait)
}
