package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"

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
	p.app.WaitFor("id:film-play", wait)
	p.app.Press("id:film-play")
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
	p.at(1500 * time.Millisecond)
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
	p.app.Press("id:player-loop")
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
	p.app.Press("id:player-loop")
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
	p, f := openPlayer(t)
	p.at(6 * sec)
	p.app.Press("id:player-back")
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(p.dir).Films()
		return len(films) == 1 && films[0].PositionMS == 6000
	})
	p.app.Press("id:film-play")
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec})
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
