package main

import (
	"strings"
	"testing"
	"time"

	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
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
	if got := p.texts("id:player-mode-pill"); got != msgs.PlayerModePill(msgs.PlayerModeBilingual()) {
		t.Fatalf("the mode pill reads %q", got)
	}
	p.app.Press("id:player-mode-pill")
	if got := p.texts("id:player-mode-pill"); got != msgs.PlayerModePill(msgs.PlayerModeOriginal()) {
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

// Where the system offers picture in picture the playback settings do; once its window shows, the player leaves for the tabs without the mini player, which the window stands in for.
func TestPictureInPictureTakesTheMiniPlayersPlace(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Press("id:player-more")
	if p.app.Exists("id:player-settings-pip") {
		t.Fatal("picture in picture is offered where the system has none")
	}
	p.app.ReportPictureInPicture("id:player-video", core.PictureInPictureState{Supported: true, Possible: true})
	p.app.WaitFor("id:player-settings-pip", wait)
	p.app.Press("id:player-settings-pip")
	p.app.ReportPictureInPicture("id:player-video", core.PictureInPictureState{Supported: true, Possible: true, Active: true})
	p.app.WaitForGone("id:player-card", wait)
	if p.app.Exists("id:player-mini") {
		t.Fatal("the mini player shows beside the picture in picture window")
	}
	p.app.ReportPictureInPicture("id:player-video", core.PictureInPictureState{Supported: true, Possible: true})
	p.app.WaitFor("id:player-mini", wait)
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
