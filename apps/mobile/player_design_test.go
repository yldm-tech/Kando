package main

import (
	"strings"
	"testing"
	"time"

	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// While the film holds, a play mark shows in the middle of the picture; playing, it goes.
func TestThePictureShowsAPlayMarkWhileTheFilmHolds(t *testing.T) {
	p, _ := openPlayer(t)
	if p.app.Exists("id:player-paused") {
		t.Fatal("the play mark shows while the film plays")
	}
	p.app.Press("id:player-play")
	p.app.WaitFor("id:player-paused", wait)
	p.app.Press("id:player-play")
	p.app.WaitForGone("id:player-paused", wait)
}

// The transport says the time left, and its speed pill writes a whole speed with its tenth.
func TestTheTransportSaysTheTimeLeft(t *testing.T) {
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	if got := p.texts("id:player-left"); got != "-0:06" {
		t.Fatalf("the time left reads %q", got)
	}
	if got := p.texts("id:player-speed"); got != "1.0×" {
		t.Fatalf("the speed pill reads %q", got)
	}
}

// On its side the phone's row over the picture says the subtitle mode, and a tap on it goes on to the next; its capsule loops the line.
func TestOnItsSideTheModePillCyclesTheModes(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Resize(844, 390)
	if got := p.texts("id:player-mode-pill"); got != msgs.PlayerModePill(msgs.PlayerModeShortBilingual()) {
		t.Fatalf("the mode pill reads %q", got)
	}
	p.app.Press("id:player-mode-pill")
	if got := p.texts("id:player-mode-pill"); got != msgs.PlayerModePill(msgs.PlayerModeShortOriginal()) {
		t.Fatalf("after a tap the mode pill reads %q", got)
	}
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-loop")
	if !p.app.Exists("id:player-pill-loop") {
		t.Fatal("the capsule's loop did not loop the line")
	}
}

// The breakdown of an explanation, a word a line, shows each word as a row, which looks the word up in the line explained.
func TestTheBreakdownsWordsLookThemselvesUp(t *testing.T) {
	d := newDesktop(t)
	d.SetEntry("how", sync.WordEntry{Word: "how", Meaning: "adv. 怎样", Language: "简体中文"})
	d.SetExplanationSections(
		sync.ExplainSection{Kind: sync.ExplainMeaning, Text: "Asks how someone is."},
		sync.ExplainSection{Kind: sync.ExplainBreakdown, Text: "how · 怎样\nyou · 你"},
	)
	p, _ := studyPlayer(t, d)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-explain-line")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-explain-word") })
	if n := len(p.app.Query("id:player-explain-word")); n != 2 {
		t.Fatalf("the breakdown shows %d rows", n)
	}
	p.app.Press("id:player-explain-word >> nth=0")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-word-meaning") })
	if p.app.Exists("id:player-explain") || p.app.Find("id:player-word-meaning").Text != "adv. 怎样" {
		t.Fatalf("the row opened %q", p.texts("id:player-word-sheet"))
	}
	// The word looked up is marked in its line on the card.
	if how := p.app.Span("id:player-card >> text:How"); how.Background.Color == 0 {
		t.Fatalf("the word looked up is not marked on the card: %+v", how)
	}
}

// A line whose translation the learner fixed says so in the card.
func TestTheCardSaysATranslationWasFixed(t *testing.T) {
	d := newDesktop(t)
	d.SetExplanation("A greeting.")
	d.SetNoGlossary(true)
	p, _ := studyPlayer(t, d)
	p.at(3200 * time.Millisecond)
	if p.app.Exists("id:player-card-fixed") {
		t.Fatal("a line not fixed says it was")
	}
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-fix", wait)
	p.app.Press("id:player-explain-fix")
	p.app.WaitFor("id:player-fix", wait)
	p.app.Clear("id:player-fix-draft")
	p.app.Type("id:player-fix-draft", "最近好吗？")
	p.app.Press("id:player-fix-save")
	p.app.WaitFor("id:player-card-fixed", wait)
	if got := p.texts("id:player-card-translation"); got != "最近好吗？" {
		t.Fatalf("the card's translation reads %q", got)
	}
}

// onIOS runs the phone as an iPhone, where NSGo starts picture in picture (core/pip.go:183-185).
var onIOS = nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS})

// Where the system offers picture in picture the playback settings do; once its window shows, the player leaves for the tabs without the mini player, which the window stands in for.
func TestPictureInPictureTakesTheMiniPlayersPlace(t *testing.T) {
	p, _ := openPlayer(t, onIOS)
	p.app.Press("id:player-more")
	if p.app.Exists("id:player-settings-pip") {
		t.Fatal("picture in picture is offered where the system has none")
	}
	p.app.ReportPictureInPicture("id:player-video", core.PictureInPictureState{Supported: true, Possible: true})
	p.app.WaitFor("id:player-settings-pip", wait)
	p.app.Press("id:player-settings-pip")
	// The row asks the video to start it; the system's window showing is what minimizes the player.
	p.app.WaitForGone("id:player-card", wait)
	if n := p.app.Find("id:player-video").PiPStarts; n != 1 {
		t.Fatalf("the row asked the video %d times to start picture in picture", n)
	}
	if p.app.Exists("id:player-mini") {
		t.Fatal("the mini player shows beside the picture in picture window")
	}
	// Back from the window to the app, the full player shows again.
	p.app.PictureInPictureControl("id:player-video", "restore", 0)
	p.app.ReportPictureInPicture("id:player-video", core.PictureInPictureState{Supported: true, Possible: true})
	p.app.WaitFor("id:player-card", wait)
	if p.app.Exists("id:player-mini") {
		t.Fatal("the mini player shows over the player restored from picture in picture")
	}
}

// Ended without a return to the app, picture in picture leaves the film in the mini player.
func TestPictureInPictureClosedLeavesTheMiniPlayer(t *testing.T) {
	p, _ := openPlayer(t, onIOS)
	p.app.ReportPictureInPicture("id:player-video", core.PictureInPictureState{Supported: true, Possible: true})
	p.app.Press("id:player-more")
	p.app.WaitFor("id:player-settings-pip", wait)
	p.app.Press("id:player-settings-pip")
	p.app.WaitForGone("id:player-card", wait)
	p.app.ReportPictureInPicture("id:player-video", core.PictureInPictureState{Supported: true, Possible: true})
	p.app.WaitFor("id:player-mini", wait)
}

// The tab bar under the card keeps its height whichever tab is chosen, and the chosen tab has its underline.
func TestThePanelsTabsKeepTheirHeight(t *testing.T) {
	p, _ := openPlayer(t)
	for _, tab := range []string{"lines", "words", "notes", "parts", "info"} {
		p.app.Press("id:player-tab-" + tab)
		p.app.WaitUntil(wait, func() bool { return p.app.Find("id:player-tab-" + tab).Props.Selected })
		for _, other := range []string{"lines", "info"} {
			if h := p.app.Find("id:player-tab-" + other).Frame.Size.Height; h != 38 {
				t.Fatalf("with %s chosen the %s tab is %v tall", tab, other, h)
			}
		}
		if h := p.app.Find("id:player-tabs").Parent.Parent.Frame.Size.Height; h < 38 {
			t.Fatalf("with %s chosen the tab bar is %v tall", tab, h)
		}
	}
}

// The transport keeps play in the middle whatever the mode's pill says.
func TestTheTransportKeepsPlayInTheMiddle(t *testing.T) {
	p, _ := openPlayer(t)
	// How far play's middle is from its row's, in the row's own coordinates.
	off := func() float32 {
		play := p.app.Find("id:player-play")
		return float32(play.Frame.Origin.X + play.Frame.Size.Width/2 - play.Parent.Frame.Size.Width/2)
	}
	for _, m := range []string{"original", "translation", "off"} {
		p.app.Press("id:player-mode")
		p.app.Press("id:player-mode-" + m)
		if d := off(); d > 1 || d < -1 {
			t.Fatalf("with the mode %s play is %v off the middle", m, d)
		}
	}
}

// Settings › Subtitle Style from the playback settings minimizes the player and opens the page in the Settings tab, once however many times it is asked.
func TestThePlaybackSettingsOpenSubtitleStyle(t *testing.T) {
	p, _ := openPlayer(t)
	for range 2 {
		p.app.Press("id:player-more")
		p.app.WaitFor("id:player-settings-style", wait)
		p.app.Press("id:player-settings-style")
		p.app.WaitFor("id:player-mini", wait)
		p.app.WaitFor("id:style-preview", wait)
		p.app.Press("id:player-mini-open")
		p.app.WaitFor("id:player-card", wait)
	}
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	if n := len(p.app.Query("id:style-preview")); n != 1 {
		t.Fatalf("Subtitle Style is open %d times", n)
	}
	p.app.Back()
	p.app.WaitForGone("id:style-preview", wait)
}

// The Info tab says where the film comes from and the language spoken in it.
func TestTheInfoTabSaysWhereTheFilmComesFrom(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Press("id:player-tab-info")
	p.app.WaitFor("id:player-info-source", wait)
	if got := p.texts("id:player-info-source"); !strings.HasPrefix(got, msgs.PlayerInfoSource()+"\n") {
		t.Fatalf("the source row reads %q", got)
	}
	if !p.app.Exists("id:player-export") {
		t.Fatal("the Info tab offers no export of the subtitles")
	}
}

// While the toast that says a word went in the vocabulary shows, Back dismisses it first, as NSGo's toasts take Back while they hold one (base/toast/toast.go:510-519); the next Back closes the word's sheet.
func TestBackAfterAddingAWordGoesToItsToastFirst(t *testing.T) {
	d := newDesktop(t)
	d.SetEntry("morning", sync.WordEntry{Word: "morning", Meaning: "n. 早晨", Language: "简体中文"})
	// The note on where lookups go was read before, so the vocabulary's toast is the only one.
	dir := t.TempDir()
	studyNoted(t, dir)
	p, _ := openStudyIn(t, d, dir)
	playStudy(p)
	p.app.Press(`id:player-card >> text:"morning"`)
	p.app.WaitFor("id:player-word-vocab", wait)
	p.app.Press("id:player-word-vocab")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerWordAdded("morning"), wait)
	p.app.Back()
	if !p.app.Exists("id:player-word-sheet") {
		t.Fatal("the first Back closed the word's sheet under the toast")
	}
	// The toast goes once its leaving has played, on the test's clock.
	p.app.Advance(time.Second)
	p.app.WaitForGone("id:player-toasts >> text:"+msgs.PlayerWordAdded("morning"), wait)
	closeWordSheet(p)
}
