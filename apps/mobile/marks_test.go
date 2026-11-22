package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
)

// talkMarks are marks of the talk's lines as a computer would compute them: a level's line under "Good", a reading over "How" and a word of the vocabulary under "you", and marks of the third line computed for another original than the phone's, which it drops.
var talkMarks = sync.FilmMarks{Lines: []sync.LineMarks{
	{Line: 0, Hash: sync.LineHash("Good morning."), Marks: []sync.Mark{{Start: 0, End: 4, Tone: sync.ToneAccent}}},
	{Line: 1, Hash: sync.LineHash("How are you?"), Marks: []sync.Mark{{Start: 0, End: 3, Ruby: "はう"}, {Start: 8, End: 11, Underline: true}}},
	{Line: 2, Hash: sync.LineHash("Fine, thank you."), Marks: []sync.Mark{{Start: 0, End: 4, Tone: sync.ToneWarn}}},
}}

// TestMarksFromTheComputerShowOnTheCard checks the word marks the computer computes for a film's lines: the phone asks for them as the film opens and keeps them with it, the card draws a level's word on a faint box in its tone, a word of the vocabulary black on yellow and a reading over its word, and a line whose original is not the one they were computed for shows none of them. Settings › Subtitle Style turns them off and on.
func TestMarksFromTheComputerShowOnTheCard(t *testing.T) {
	d := newDesktop(t)
	d.SetMarks("f1", talkMarks)
	p, f := studyPlayer(t, d)
	p.app.WaitUntil(wait, func() bool {
		_, err := os.Stat(filepath.Join(p.dir, "films", f.Desktop, f.Film.ID, "marks.json"))
		return err == nil
	})
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:player-card >> text:Good") && p.app.Span("id:player-card >> text:Good").Background.Color != 0
	})
	// On the card a level's word is a highlighter's faint band in its tone over the lower third of its line, rather than a line under it.
	if good := p.app.Span("id:player-card >> text:Good"); good.Style.Decoration != 0 || good.Background.Color == theme.SubtitleYellow || good.Background.Top != core.Pct(62) {
		t.Fatalf("the level's mark on Good: %+v", good)
	}
	// An unmarked word is plain, a tap on it still looking it up.
	if morning := p.app.Span("id:player-card >> text:morning"); morning.Style.Decoration != 0 || morning.Background.Color != 0 {
		t.Fatalf("the word without a mark: %+v", morning)
	}

	p.at(3200 * time.Millisecond)
	p.app.WaitFor("id:player-card >> id:player-ruby >> text:はう", wait)
	// A word of the vocabulary is over a band of the subtitles' yellow, the lower 38% of its line, in the line's own ink.
	if you := p.app.Span("id:player-card >> text:you"); you.Background.Color != theme.SubtitleYellow || you.Background.Top != core.Pct(62) || you.Style.Color == core.RGB(0x000000) {
		t.Fatalf("the word of the vocabulary: %+v", you)
	}

	p.at(5500 * time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-card-original") == "Fine, thanks." })
	if fine := p.app.Span("id:player-card >> text:Fine"); fine.Background.Color != 0 || fine.Style.Decoration != 0 {
		t.Fatalf("marks computed for another original show: %+v", fine)
	}
	if d.MarksAsked() == 0 {
		t.Fatal("the phone did not ask the computer")
	}
}

// TestMarksStayWithTheFilmOffline checks a film played away from its computer: the marks kept with it show, as the computer last answered them.
func TestMarksStayWithTheFilmOffline(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	if err := store.Open(dir).SaveMarks(f.Desktop, f.Film.ID, talkMarks); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(3200 * time.Millisecond)
	p.app.WaitFor("id:player-card >> id:player-ruby >> text:はう", wait)
}

// TestMarksStayWithAComputerFromBeforeThem checks a computer that answers no marks, as a Kando from before them: the marks kept with the film stay.
func TestMarksStayWithAComputerFromBeforeThem(t *testing.T) {
	d := newDesktop(t)
	d.SetNoMarks(true)
	p, dir, f := openWithMarks(t, d)
	p.app.WaitFor("id:player-card >> id:player-ruby >> text:はう", wait)
	if m, ok, _ := store.Open(dir).Marks(f.Desktop, f.Film.ID); !ok || len(m.Lines) != 3 {
		t.Fatalf("the marks kept are %+v", m)
	}
}

// TestMarksGoWhenTheComputerMarksNothing checks a computer that marks nothing on the film any more, its switches turned off: the phone drops the marks it kept, on the card and on disk.
func TestMarksGoWhenTheComputerMarksNothing(t *testing.T) {
	d := newDesktop(t)
	d.SetMarks("f1", sync.FilmMarks{})
	p, dir, f := openWithMarks(t, d)
	p.app.WaitUntil(wait, func() bool {
		m, ok, _ := store.Open(dir).Marks(f.Desktop, f.Film.ID)
		return ok && len(m.Lines) == 0
	})
	p.app.WaitUntil(wait, func() bool {
		return !p.app.Exists("id:player-ruby") && p.texts("id:player-card-original") == "How are you?"
	})
}

// openWithMarks opens the talk kept from d with talkMarks in the player, at its second line.
func openWithMarks(t *testing.T, d *synctest.Desktop) (*phone, string, store.Film) {
	t.Helper()
	dir := t.TempDir()
	f := keepTalkFrom(t, dir, store.DesktopID(d.Fingerprint))
	if err := store.Open(dir).SaveMarks(f.Desktop, f.Film.ID, talkMarks); err != nil {
		t.Fatal(err)
	}
	p, _ := openStudyIn(t, d, dir)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(3200 * time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return d.MarksAsked() > 0 || p.app.Exists("id:player-ruby") })
	return p, dir, f
}

// TestMarksOnThePicture checks the marks on the line over the picture, as a phone on its side shows it: the reading over its word and the word of the vocabulary black on yellow.
func TestMarksOnThePicture(t *testing.T) {
	d := newDesktop(t)
	d.SetMarks("f1", talkMarks)
	p, _ := studyPlayer(t, d)
	p.app.Resize(844, 390)
	p.at(3200 * time.Millisecond)
	p.app.WaitFor("id:player-subtitles >> id:player-ruby >> text:はう", wait)
	if you := p.app.Span("id:player-subtitles >> text:you"); you.Background.Color != theme.SubtitleYellow {
		t.Fatalf("the word of the vocabulary on the picture: %+v", you)
	}
}

// TestMarksTurnOffInSubtitleStyle checks the switch of Settings › Subtitle Style: turned off, it is kept with the phone's preferences, and the player shows no marks.
func TestMarksTurnOffInSubtitleStyle(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	if err := store.Open(dir).SaveMarks(f.Desktop, f.Film.ID, talkMarks); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-style")
	p.app.WaitFor("id:style-marks-switch", wait)
	if !p.app.Find("id:style-marks-switch").Props.Checked {
		t.Fatal("the marks are off by default")
	}
	p.app.Press("id:style-marks-switch")
	p.app.WaitUntil(wait, func() bool {
		prefs, err := store.Open(dir).Prefs()
		return err == nil && prefs.Subtitles == store.SubStyle{NoMarks: true}
	})
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(3200 * time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-card-original") == "How are you?" })
	if p.app.Exists("id:player-ruby") || p.app.Span("id:player-card >> text:you").Background.Color == theme.SubtitleYellow {
		t.Fatal("the marks show once turned off")
	}
}

// TestMarksFollowTheComputersNewLines checks a film whose lines changed on its computer since the phone kept it, as one proofread or one whose subtitles are made while it plays: once the player takes the new lines, the phone asks the computer for their marks again, and draws those of the new original.
func TestMarksFollowTheComputersNewLines(t *testing.T) {
	d := desktopWith(t, revisedCues, now().Add(time.Hour))
	d.SetMarks("f1", sync.FilmMarks{Lines: []sync.LineMarks{{Line: 0, Hash: sync.LineHash("Good morning!"), Marks: []sync.Mark{{Start: 0, End: 4, Ruby: "ぐっど"}}}}})
	dir, _ := keptFrom(t, d)
	p := playKept(t, d, dir)
	// The new lines show, with the marks of the new original, which the phone asked for again once it took them.
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-card-translation") == "早安！" })
	p.app.WaitUntil(wait, func() bool { return d.MarksAsked() >= 2 })
	p.app.WaitFor("id:player-card >> id:player-ruby >> text:ぐっど", wait)
}

// TestMarksAskAgainWhileTheComputerComputesThem checks a computer still computing a film's marks: the phone asks again, a while apart, up to its tries, and keeps the marks it had.
func TestMarksAskAgainWhileTheComputerComputesThem(t *testing.T) {
	d := newDesktop(t)
	d.SetMarksPending(true)
	p, dir, f := openWithMarks(t, d)
	began := time.Now()
	p.app.WaitUntil(wait, func() bool { return d.MarksAsked() >= 3 })
	if took := time.Since(began); took < time.Second {
		t.Fatalf("the phone asked three times in %v", took)
	}
	if m, ok, _ := store.Open(dir).Marks(f.Desktop, f.Film.ID); !ok || len(m.Lines) != 3 {
		t.Fatalf("the marks kept are %+v", m)
	}
	p.app.WaitFor("id:player-card >> id:player-ruby >> text:はう", wait)
}
