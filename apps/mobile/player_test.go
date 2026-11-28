package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

const sec = time.Second

// openPlayer opens the talk kept on the phone in the player, offline, and loads its video.
func openPlayer(t *testing.T, opts ...nsgotest.Option) (*phone, store.Film) {
	t.Helper()
	dir := t.TempDir()
	f := keepTalk(t, dir)
	p := startPhone(t, dir, opts...)
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") })
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	return p, f
}

// at reports the video played to at.
func (p *phone) at(at time.Duration) { p.app.Progress("id:player-video", at, 10*sec) }

// seeks are the positions the player asked the video to jump to.
func (p *phone) seeks() []time.Duration { return p.app.Find("id:player-video").Seeks }

func TestThePlayerDrawsTheLinePlayingAsTheModePicks(t *testing.T) {
	p, f := openPlayer(t)
	if src := p.app.Find("id:player-video").Video.Source; filepath.Base(src) != f.Video || !strings.Contains(src, f.Desktop) {
		t.Fatalf("the video plays %q", src)
	}
	if p.app.Find("id:player-video").Video.Paused {
		t.Fatal("the film is held as it opens")
	}
	// Upright the lines show in the card alone, until the settings put them on the picture too.
	p.at(1500 * time.Millisecond)
	if p.app.Exists("id:player-subtitles") {
		t.Fatal("upright, the lines show on the picture by default")
	}
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-on-picture")
	if got := p.texts("id:player-subtitles"); got != "早上好。\nGood morning." {
		t.Fatalf("bilingual: %q", got)
	}
	p.app.Press("id:player-mode")
	p.app.Press("id:player-mode-original")
	if got := p.texts("id:player-subtitles"); got != "Good morning." {
		t.Fatalf("original only: %q", got)
	}
	p.app.Press("id:player-mode")
	p.app.Press("id:player-mode-translation")
	if got := p.texts("id:player-subtitles"); got != "早上好。" {
		t.Fatalf("translation only: %q", got)
	}
	// A line without a translation shows its original in its place.
	p.at(5500 * time.Millisecond)
	if got := p.texts("id:player-subtitles"); got != "Fine, thanks." {
		t.Fatalf("translation only, untranslated: %q", got)
	}
	// Between two lines nothing shows.
	p.at(2500 * time.Millisecond)
	if p.app.Exists("id:player-subtitles") && p.texts("id:player-subtitles") != "" {
		t.Fatalf("between lines: %q", p.texts("id:player-subtitles"))
	}
}

func TestThePlayerLoopsALineAndStepsBetweenLines(t *testing.T) {
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-card-loop")
	before := len(p.seeks())
	p.at(4000 * time.Millisecond)
	if len(p.seeks()) != before {
		t.Fatalf("the loop jumped inside its line: %v", p.seeks())
	}
	p.at(4600 * time.Millisecond)
	if s := p.seeks(); len(s) != before+1 || s[len(s)-1] != 3*sec {
		t.Fatalf("at the line's end the loop went to %v, want 3s", s)
	}
	// The next line moves the loop along with it.
	p.app.Press("id:player-next")
	if s := p.seeks(); s[len(s)-1] != 5*sec {
		t.Fatalf("next line went to %v, want 5s", s)
	}
	p.at(6600 * time.Millisecond)
	if s := p.seeks(); s[len(s)-1] != 5*sec || len(s) != before+3 {
		t.Fatalf("the loop of the next line went to %v", s)
	}
	p.app.Press("id:player-prev")
	if s := p.seeks(); s[len(s)-1] != 3*sec {
		t.Fatalf("the line before went to %v, want 3s", s)
	}
	p.app.Press("id:player-card-loop")
	n := len(p.seeks())
	p.at(4600 * time.Millisecond)
	if len(p.seeks()) != n {
		t.Fatalf("the loop went on after it was turned off: %v", p.seeks())
	}
}

func TestTheLineListMarksTheLinePlayingAndPlaysTheOneTapped(t *testing.T) {
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	rows := p.app.Query("id:player-line")
	if len(rows) != 4 {
		t.Fatalf("%d lines listed, want 4", len(rows))
	}
	for i, r := range rows {
		if r.Props.Selected != (i == 1) {
			t.Errorf("line %d selected %v while the second plays", i, r.Props.Selected)
		}
	}
	if got := p.texts("id:player-line >> nth=3"); !strings.Contains(got, "再见。") || !strings.Contains(got, "See you.") || !strings.Contains(got, "0:07") {
		t.Errorf("the fourth line reads %q", got)
	}
	p.app.Press("id:player-line >> nth=3")
	if s := p.seeks(); s[len(s)-1] != 7*sec {
		t.Fatalf("tapping the fourth line went to %v, want 7s", s)
	}
}

// keptPosition is the position the film's record on disk holds, -1 for none read.
func (p *phone) keptPosition() int64 {
	films, err := store.Open(p.dir).Films()
	if err != nil || len(films) != 1 {
		return -1
	}
	return films[0].PositionMS
}

// Positions kept one after another, as a film seeked back and forth, are written in the order they were kept: the record ends with the last, not with an older one written after it.
func TestPositionsKeptOneAfterAnotherEndWithTheLast(t *testing.T) {
	p, _ := openPlayer(t)
	for range 20 {
		p.at(9 * sec)
		p.at(1 * sec)
	}
	p.at(7 * sec)
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 7000 })
	time.Sleep(300 * time.Millisecond)
	if got := p.keptPosition(); got != 7000 {
		t.Fatalf("the record ended with %d, want 7000", got)
	}
}

// A position that could not be written, as with the film's folder not writable for a while, is written once it can be, though the player does not ask again: the in-memory record already holds it.
func TestAPositionThatCouldNotBeWrittenIsWrittenOnceItCan(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a folder's mode keeps files out of it only for a user other than root, on a system other than Windows")
	}
	p, f := openPlayer(t)
	folder := store.Open(p.dir).FilmDir(f.Desktop, f.Film.ID)
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })
	p.at(6 * sec)
	// The first write fails while the folder is read-only.
	time.Sleep(300 * time.Millisecond)
	if got := p.keptPosition(); got == 6000 {
		t.Fatalf("the position was written into a read-only folder")
	}
	if err := os.Chmod(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 6000 })
}

func TestThePlayerKeepsWhereTheFilmWasLeft(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	play := func(t *testing.T) *phone {
		p := startPhone(t, dir)
		p.app.WaitFor(talkRow(f.Desktop), wait)
		p.app.Press(talkRow(f.Desktop))
		p.app.WaitFor("id:player-video", wait)
		p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec})
		return p
	}
	t.Run("before", func(t *testing.T) {
		p := play(t)
		p.at(6 * sec)
		p.app.Press("id:player-minimize")
		p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 6000 })
	})
	// The app started again plays the film from where it was left.
	p := play(t)
	if s := p.seeks(); !slices.Contains(s, 6*sec) {
		t.Fatalf("reopened, %s went to %v, not where it was left", f.Film.Title, s)
	}
}

func TestThePlayerChangesSpeed(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Press("id:player-speed")
	p.app.Press("id:player-speed-1.5×")
	if got := p.app.Find("id:player-video").Video.Rate; got != 1.5 {
		t.Fatalf("rate %v, want 1.5", got)
	}
	p.app.Press("id:player-play")
	if !p.app.Find("id:player-video").Video.Paused {
		t.Fatal("the play button did not hold the film")
	}
}

// On its side a phone gives the video the whole screen, the lines out of the way and the controls over it at a tap; upright again, the lines are back.
func TestThePlayerFillsTheScreenOnItsSide(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Resize(844, 390)
	if p.app.Exists("id:player-lines") {
		t.Fatal("the line list shows beside a full-screen video")
	}
	stage := p.app.Find("id:player-stage").Frame
	if stage.Size.Width != 844 || stage.Size.Height != 390 {
		t.Fatalf("the video takes %v of the 844x390 screen", stage.Size)
	}
	if !p.app.Exists("id:player-transport") {
		t.Fatal("the controls do not show as the phone turns")
	}
	p.app.Press("id:player-video")
	if p.app.Exists("id:player-transport") {
		t.Fatal("a tap on the video did not hide the controls")
	}
	p.app.Resize(390, 844)
	if !p.app.Exists("id:player-lines") || !p.app.Exists("id:player-transport") {
		t.Fatal("upright again, the lines or the transport are gone")
	}
}

// A tablet on its side shows the lines in a column beside the video.
func TestATabletShowsTheLinesBesideTheVideo(t *testing.T) {
	p, _ := openPlayer(t, nsgotest.Size(1180, 820))
	pane, stage := p.app.Find("id:player-lines-pane").Frame, p.app.Find("id:player-stage").Frame
	if pane.Size.Width != 380 || pane.Origin.X < stage.Size.Width {
		t.Fatalf("lines %v, video %v", pane, stage)
	}
}

// The position slider runs over the video's length once the video tells it, the same slider as before it knew.
func TestThePositionSliderSpansTheVideo(t *testing.T) {
	p, _ := openPlayer(t)
	slider := p.app.Find("id:player-position-thumb")
	if got := slider.Props.Value; got.Max != 10 || got.Min != 0 {
		t.Fatalf("the slider spans %v to %v, want 0 to 10 seconds", got.Min, got.Max)
	}
}

// positionAt is the position record of film f1 the computer d keeps for library lib, in milliseconds, -1 for none.
func positionAt(d *synctest.Desktop) float64 {
	r, ok := d.State("lib", sync.KindPosition, "f1")
	if !ok {
		return -1
	}
	var ms float64
	if json.Unmarshal(r.Fields[sync.FieldPositionMS].Value, &ms) != nil {
		return -1
	}
	return ms
}

// A computer's film streamed from it, which the phone never played, opens where the computer left it, as Continue Watching says, not at the beginning; the video's reports from the start of the file before its seek lands keep nothing, so the beginning does not go over the computer's place, and the place it plays from is kept once it plays from there.
func TestAStreamedFilmOpensWhereTheComputerLeftIt(t *testing.T) {
	d := newDesktop(t)
	d.SetState("lib", now().Add(-time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(6000)}}})
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor("id:films-continue-"+id+"-f1", wait)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:film-stream", wait)
	p.app.Press("id:film-stream")
	p.app.WaitFor("id:player-video", wait)
	// The video starts at the beginning of the file and says so before it is asked to seek, and once more before the seek lands.
	p.at(0)
	p.at(300 * time.Millisecond)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec})
	if s := p.seeks(); len(s) == 0 || s[len(s)-1] != 6*sec {
		t.Fatalf("the film went to %v, want where the computer left it, 6s", s)
	}
	p.at(500 * time.Millisecond)
	if got := p.texts("id:player-at"); got != "0:06" {
		t.Fatalf("before the seek lands the playhead says %q, want 0:06", got)
	}
	p.app.Press("id:player-minimize")
	time.Sleep(300 * time.Millisecond)
	if films, err := store.Open(dir).Films(); err != nil || len(films) != 1 || films[0].PositionMS != 0 || !films[0].PositionAt.IsZero() {
		t.Fatalf("before the seek landed the phone kept %+v, %v", films, err)
	}
	if got := positionAt(d); got != 6000 {
		t.Fatalf("before the seek landed the computer's place became %v, want 6000", got)
	}
	// Once the video plays from there, where it plays is the film's place, the phone's and the computer's.
	p.app.Press("id:player-mini-open")
	p.at(6200 * time.Millisecond)
	p.at(9500 * time.Millisecond)
	p.app.Press("id:player-minimize")
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 9500 && positionAt(d) == 9500 })
}

// A place the phone left the film at later than the computer's is where it opens.
func TestTheNewerPlaceOnThePhoneWins(t *testing.T) {
	d := newDesktop(t)
	d.SetState("lib", now().Add(-time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(4000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	f := keepTalkFrom(t, dir, id)
	f.PositionMS, f.PositionAt = 7000, now()
	if err := store.Open(dir).SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:films-continue-"+id+"-f1", wait)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec})
	if s := p.seeks(); len(s) == 0 || s[len(s)-1] != 7*sec {
		t.Fatalf("the film went to %v, want where the phone left it later, 7s", s)
	}
}

// A film whose computer is out of reach, so that the Library lists it from the phone alone, opens where the computer's learning state, as the phone last synced it, left it later than the phone did.
func TestAFilmOpensWhereTheComputerLeftItWhileTheComputerIsAway(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	d.Close()
	f := keepTalkFrom(t, dir, id)
	f.LibraryID = "lib"
	st := store.Open(dir)
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	pos := sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(4000), Stamp: sync.Stamp{At: now().Add(time.Hour), Device: "desktop"}}}}
	// A line saved from the film, due for review, tells the test once the learning state is read.
	line := study.Save("l1", "f1", "Morning Talk", study.Cue{Start: sec, End: 2 * sec, Original: "Good morning.", Translation: "早上好。"}, "", now())
	if err := st.SaveLearning(id, "lib", study.Library{Epoch: "e", Rev: 1, Records: []sync.Record{pos, line}}); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, secrets)
	p.app.WaitFor(talkRow(id), wait)
	p.app.WaitFor("id:bar-badge-tab-learn", wait)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec})
	if s := p.seeks(); len(s) == 0 || s[len(s)-1] != 4*sec {
		t.Fatalf("the film went to %v, want where the computer last left it, 4s", s)
	}
}

// A film left within the last two seconds of its end opens from the beginning, as before.
func TestAFilmLeftAtItsEndStartsAgain(t *testing.T) {
	d := newDesktop(t)
	d.SetState("lib", now().Add(time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(9000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepTalkFrom(t, dir, id)
	p := startPhone(t, dir, secrets)
	p.app.WaitFor(talkRow(id), wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists(talkRow(id) + " >> text:YouTube · 0:10 · " + msgs.FilmDownloaded()) })
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec})
	if s := p.seeks(); slices.Contains(s, 9*sec) {
		t.Fatalf("a film left at its end went to %v, want the beginning", s)
	}
}
