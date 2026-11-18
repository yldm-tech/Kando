package main

import (
	"encoding/json"
	"fmt"
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

// studyPlayer opens the talk in the player, kept on the phone as downloaded from d, which is paired and serves it in its library "lib", and plays it to the middle of its first line.
func studyPlayer(t *testing.T, d *synctest.Desktop) (*phone, store.Film) {
	t.Helper()
	p, f := openStudy(t, d)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(1500 * time.Millisecond)
	return p, f
}

// openStudy opens the talk in the player as studyPlayer does, once its library is known, without reporting the video's playing, which only headless tests can: on a target its card shows the first line.
func openStudy(t *testing.T, d *synctest.Desktop) (*phone, store.Film) {
	t.Helper()
	return openStudyIn(t, d, t.TempDir())
}

// openStudyIn is openStudy with the phone's data in dir, which may hold what the phone kept before.
func openStudyIn(t *testing.T, d *synctest.Desktop, dir string) (*phone, store.Film) {
	t.Helper()
	f := keepTalkFrom(t, dir, store.DesktopID(d.Fingerprint))
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:film-play", wait)
	p.app.Press("id:film-play")
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") })
	// The film's library is asked for as it opens, as the talk was kept without it.
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(dir).Films()
		return len(films) == 1 && films[0].LibraryID == "lib"
	})
	return p, f
}

// The card shows the line playing with its place among the film's lines; Hidden blurs both its lines, and a tap shows the one tapped until the next line plays.
func TestTheCardShowsTheLineAndHidesItAsTheModeSays(t *testing.T) {
	p, _ := openPlayer(t)
	p.at(3200 * time.Millisecond)
	if got := p.app.Find("id:player-card-place").Text; got != msgs.PlayerCardPlace(2, 4, "0:03") {
		t.Fatalf("the card's place reads %q", got)
	}
	if p.texts("id:player-card-original") != "How are you?" || p.texts("id:player-card-translation") != "你好吗？" {
		t.Fatalf("the card reads %q / %q", p.texts("id:player-card-original"), p.texts("id:player-card-translation"))
	}
	// Between two lines the card keeps the one just heard.
	p.at(4700 * time.Millisecond)
	if p.texts("id:player-card-original") != "How are you?" {
		t.Fatalf("between lines the card reads %q", p.texts("id:player-card-original"))
	}
	p.app.Press("id:player-mode")
	p.app.Press("id:player-mode-off")
	for _, id := range []string{"id:player-card-original", "id:player-card-translation"} {
		if got := p.app.Find(id).Props.Label; got != msgs.PlayerHiddenLine() {
			t.Fatalf("hidden, %s reads %q", id, got)
		}
	}
	p.app.Press("id:player-card-translation")
	if got := p.app.Find("id:player-card-translation").Props.Label; got != "你好吗？" {
		t.Fatalf("tapped, the translation reads %q", got)
	}
	p.at(5200 * time.Millisecond)
	if got := p.app.Find("id:player-card-original").Props.Label; got != msgs.PlayerHiddenLine() {
		t.Fatalf("the next line shows %q", got)
	}
	// Original Only hides the translation alone.
	p.app.Press("id:player-mode")
	p.app.Press("id:player-mode-original")
	p.at(3200 * time.Millisecond)
	if p.app.Find("id:player-card-original").Props.Label != "How are you?" || p.app.Find("id:player-card-translation").Props.Label != msgs.PlayerHiddenLine() {
		t.Fatal("Original Only does not hide the translation alone")
	}
}

// Replay plays the line again from its start, and Previous Line plays the line playing again a moment into it, the one before at its start.
func TestReplayAndPreviousLine(t *testing.T) {
	p, _ := openPlayer(t)
	p.at(4 * sec)
	p.app.Press("id:player-replay")
	if s := p.seeks(); s[len(s)-1] != 3*sec {
		t.Fatalf("replay went to %v", s)
	}
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-prev")
	if s := p.seeks(); s[len(s)-1] != 1*sec {
		t.Fatalf("previous line at the start of a line went to %v", s)
	}
	p.at(8700 * time.Millisecond)
	p.app.Press("id:player-prev")
	if s := p.seeks(); s[len(s)-1] != 7*sec {
		t.Fatalf("previous line well into a line went to %v", s)
	}
}

// A word tapped on the card is looked up through the film's computer, in its line, while the film holds; the same word looked up again answers on the phone, without asking.
func TestAWordTappedIsLookedUpThroughTheComputer(t *testing.T) {
	d := newDesktop(t)
	d.SetEntry("morning", sync.WordEntry{Word: "morning", Reading: "/ˈmɔːnɪŋ/", Meaning: "n. 早晨", Language: "简体中文"})
	p, _ := studyPlayer(t, d)
	p.app.Press(`id:player-card >> text:"morning"`)
	if !p.app.Find("id:player-video").Video.Paused {
		t.Fatal("the film plays on under the word")
	}
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-word-meaning") })
	if p.app.Find("id:player-word-meaning").Text != "n. 早晨" || p.app.Find("id:player-word-reading").Text != "/ˈmɔːnɪŋ/" || p.app.Find("id:player-word-line").Text != "Good morning." {
		t.Fatalf("the word card reads %q", p.texts("id:player-word-sheet"))
	}
	got := d.Lookups()
	if len(got) != 1 || got[0].Word != "morning" || got[0].Line != "Good morning." || got[0].Film != "f1" || got[0].Library != "lib" {
		t.Fatalf("the computer was asked %+v", got)
	}
	p.app.Press("id:player-word-replay")
	if s := p.seeks(); s[len(s)-1] != 1*sec || p.app.Find("id:player-video").Video.Paused {
		t.Fatalf("replay line went to %v", s)
	}
	p.app.Press(`id:player-card >> text:"morning"`)
	p.app.WaitFor("id:player-word-meaning", wait)
	if len(d.Lookups()) != 1 {
		t.Fatal("the word looked up again asked the computer")
	}
}

// Without an AI service on the computer, the word card says to set one up there.
func TestALookupWithoutAnAIServiceSaysWhere(t *testing.T) {
	d := newDesktop(t)
	d.SetNoAI(true)
	p, _ := studyPlayer(t, d)
	p.app.Press(`id:player-card >> text:"Good"`)
	p.app.WaitFor("id:player-word-problem", wait)
	if got := p.app.Find("id:player-word-problem").Text; got != msgs.PlayerStudyNoAI("Studio") {
		t.Fatalf("the word card says %q", got)
	}
}

// Explain sends the line with the lines around it to the computer, whose AI service's explanation the sheet shows with the model and the computer.
func TestExplainAsksTheComputer(t *testing.T) {
	d := newDesktop(t)
	d.SetExplanation("A greeting for the morning.")
	p, _ := studyPlayer(t, d)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-text", wait)
	if got := p.app.Find("id:player-explain-text").Text; got != "A greeting for the morning." {
		t.Fatalf("the explanation reads %q", got)
	}
	if got := p.app.Find("id:player-explain-by").Text; got != msgs.PlayerExplainBy("test-model", "Studio") {
		t.Fatalf("the byline reads %q", got)
	}
	x := d.Explains()
	if len(x) != 1 || x[0].Index != 1 || len(x[0].Lines) != 3 || x[0].Lines[1].Original != "How are you?" || x[0].Lines[1].StartMS != 3000 {
		t.Fatalf("the computer was asked %+v", x)
	}
}

// fieldText is field name of r as a string.
func fieldText(r sync.Record, name string) string {
	var s string
	_ = json.Unmarshal(r.Fields[name].Value, &s)
	return s
}

// A line saved on the phone goes to the computer, with the note written on it; one saved on the computer shows on the phone; a line taken back is gone on the computer too.
func TestSavedLinesAndNotesSyncWithTheComputer(t *testing.T) {
	d := newDesktop(t)
	at := now().Add(-time.Hour)
	field := func(v any) sync.Field { return sync.Field{Value: sync.Value(v)} }
	d.SetState("lib", at, sync.Record{Kind: sync.KindLine, ID: "desk1", Film: "f1", Fields: map[string]sync.Field{
		sync.FieldStartMS: field(7000), sync.FieldEndMS: field(9000), sync.FieldOriginal: field("See you."), sync.FieldTranslation: field("再见。"),
		sync.FieldTitle: field("Morning Talk"), sync.FieldSavedAt: field(at), sync.FieldNote: field("from the computer"), sync.FieldReview: field(sync.Review{Due: at}),
	}})
	p, _ := studyPlayer(t, d)
	// The computer's line shows starred in the list and in the notes.
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line-saved") })
	p.app.Press("id:player-tab-notes")
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:player-saved-note") && p.app.Find("id:player-saved-note").Text == "from the computer"
	})

	p.app.Press("id:player-save")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerSavedToast(), wait)
	saved := func() (sync.Record, bool) {
		for _, r := range d.Records("lib", sync.KindLine) {
			if r.ID != "desk1" && r.Removed == nil {
				return r, true
			}
		}
		return sync.Record{}, false
	}
	p.app.WaitUntil(wait, func() bool { _, ok := saved(); return ok })
	r, _ := saved()
	if r.Film != "f1" || fieldText(r, sync.FieldOriginal) != "Good morning." || fieldText(r, sync.FieldTitle) != "Morning Talk" || string(r.Fields[sync.FieldStartMS].Value) != "1000" {
		t.Fatalf("the computer has %+v", r)
	}

	p.app.Type("id:player-note", "a greeting")
	p.app.Press("id:player-note-save")
	p.app.WaitUntil(wait, func() bool { r, ok := saved(); return ok && fieldText(r, sync.FieldNote) == "a greeting" })
	if !slices.ContainsFunc(strings.Split(p.texts("id:player-notes"), "\n"), func(s string) bool { return s == "a greeting" }) {
		t.Fatalf("the notes read %q", p.texts("id:player-notes"))
	}

	p.app.Press("id:player-save")
	p.app.WaitUntil(wait, func() bool {
		for _, rec := range d.Records("lib", sync.KindLine) {
			if rec.ID == r.ID {
				return rec.Removed != nil
			}
		}
		return false
	})
}

// Shadowing holds the film after each line, says so over the picture, and plays on when played; the sleep timer set to the line's end holds it there and turns itself off.
func TestShadowingAndTheSleepTimerHoldAfterALine(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-follow")
	p.at(1200 * time.Millisecond)
	p.at(2100 * time.Millisecond)
	if !p.app.Find("id:player-video").Video.Paused {
		t.Fatal("shadowing did not hold the film after the line")
	}
	if got := p.texts("id:player-pill-follow"); got != msgs.PlayerFollowHeld() {
		t.Fatalf("the pill reads %q", got)
	}
	p.app.Press("id:player-play")
	if p.app.Find("id:player-video").Video.Paused || p.texts("id:player-pill-follow") != msgs.PlayerFollow() {
		t.Fatal("play did not go on")
	}
	p.app.Press("id:player-settings-follow")
	p.app.Press("id:player-settings-sleep-line")
	p.at(3200 * time.Millisecond)
	if p.texts("id:player-pill-sleep") != msgs.PlayerSleepPillLine() {
		t.Fatalf("the sleep pill reads %q", p.texts("id:player-pill-sleep"))
	}
	p.at(4600 * time.Millisecond)
	if !p.app.Find("id:player-video").Video.Paused || p.app.Exists("id:player-pill-sleep") {
		t.Fatal("the sleep timer did not hold the film after the line and turn off")
	}
}

// The lines moved later for a film show later against its picture, and the film keeps how far.
func TestSubtitleTimingMovesTheLinesForTheFilm(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Press("id:player-more")
	for range 5 {
		p.app.Press("id:player-delay-later")
	}
	if got := p.app.Find("id:player-delay").Text; got != msgs.PlayerDelaySeconds("+0.5") {
		t.Fatalf("the timing reads %q", got)
	}
	p.at(3200 * time.Millisecond)
	if got := p.app.Find("id:player-card-place").Text; got != msgs.PlayerCardPlace(1, 4, "0:01") {
		t.Fatalf("0.2 s into the second line, moved half a second later, the card shows %q", got)
	}
	p.at(3600 * time.Millisecond)
	if got := p.app.Find("id:player-card-place").Text; got != msgs.PlayerCardPlace(2, 4, "0:03") {
		t.Fatalf("the card shows %q", got)
	}
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(p.dir).Films()
		return len(films) == 1 && films[0].DelayMS == 500
	})
	p.app.Press("id:player-delay")
	if got := p.app.Find("id:player-delay").Text; got != msgs.PlayerDelaySeconds("0.0") {
		t.Fatalf("reset, the timing reads %q", got)
	}
}

// The Parts tab cuts the film into parts of equal time and plays from the first line of the one tapped.
func TestThePartsTabPlaysFromAPart(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Press("id:player-tab-parts")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-part") })
	if n := len(p.app.Query("id:player-part")); n != 3 {
		t.Fatalf("%d parts, want 3: the last quarter has no line starting in it", n)
	}
	p.app.Press("id:player-part >> nth=1")
	if s := p.seeks(); s[len(s)-1] != 3*sec {
		t.Fatalf("the second part played from %v", s)
	}
}

// On its side the phone shows the lines over the picture, whose words a tap looks up in a card at the picture's corner rather than a sheet, and slides the list of lines in at its side.
func TestOnItsSideAWordShowsInACardAndTheLinesSlideIn(t *testing.T) {
	d := newDesktop(t)
	p, _ := studyPlayer(t, d)
	p.app.Resize(844, 390)
	if p.app.Exists("id:player-card") {
		t.Fatal("on its side the phone shows the card")
	}
	if !p.app.Exists("id:player-subtitles") {
		t.Fatal("on its side the lines are not on the picture")
	}
	if p.app.Exists("id:player-lines-pane") {
		t.Fatal("the list shows before it was asked for")
	}
	p.app.Press("id:player-panel-toggle")
	if got := p.app.Find("id:player-lines-pane").Frame.Size.Width; got != 300 {
		t.Fatalf("the list is %v wide", got)
	}
	p.app.Press(`id:player-subtitles >> text:"morning"`)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-word-popover >> id:player-word-meaning") })
	if p.app.Exists("id:player-word-sheet") {
		t.Fatal("on its side the word shows in a sheet too")
	}
	p.app.Press("id:player-word-close")
	if p.app.Exists("id:player-word-popover") {
		t.Fatal("× left the card open")
	}
}

// On a tablet on its side the panel of tabs takes a column beside the picture and the card.
func TestOnATabletThePanelSitsBeside(t *testing.T) {
	p, _ := openPlayer(t, nsgotest.Size(1180, 820))
	pane := p.app.Find("id:player-lines-pane").Frame
	if pane.Size.Width != 380 || !p.app.Exists("id:player-tabs") || !p.app.Exists("id:player-card") {
		t.Fatalf("the pane is %+v", pane)
	}
}

// A line taken back while its save is still on its way to the computer is taken back there too, once the computer has it, and stays taken back on the phone.
func TestALineTakenBackWhileItsSaveIsSentIsGoneOnTheComputer(t *testing.T) {
	d := newDesktop(t)
	p, _ := studyPlayer(t, d)
	release := d.HoldPuts()
	p.app.Press("id:player-save")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line-saved") })
	p.app.Press("id:player-save")
	p.app.WaitUntil(wait, func() bool { return !p.app.Exists("id:player-line-saved") })
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerUnsavedToast(), wait)
	release()
	p.app.WaitUntil(wait, func() bool {
		lines := d.Records("lib", sync.KindLine)
		return len(lines) == 1 && lines[0].Removed != nil
	})
	p.app.WaitUntil(wait, func() bool {
		l, _ := store.Open(p.dir).Learning(store.DesktopID(d.Fingerprint), "lib")
		return len(l.Pending) == 0 && len(l.Records) == 1
	})
	if p.app.Exists("id:player-line-saved") {
		t.Fatal("the line taken back shows saved again")
	}
}

// Lines saved away from the computer, more than one call carries, all reach it once it can be reached, but for the one of a film it no longer has, which it refuses and the phone drops alone.
func TestManyLinesSavedAwayAllReachTheComputerButThoseItRefuses(t *testing.T) {
	d := newDesktop(t)
	d.RefuseState(func(r sync.Record) bool { return r.Film == "gone" })
	dir := t.TempDir()
	desktop := store.DesktopID(d.Fingerprint)
	var l study.Library
	long := strings.Repeat("a long line of the film, ", 40)
	const saved = 100
	for i := range saved {
		c := study.Cue{Start: time.Duration(i) * sec, End: time.Duration(i)*sec + 500*time.Millisecond, Original: long, Translation: long}
		l = study.Change(l, study.Save(fmt.Sprintf("away%03d", i), "f1", "Morning Talk", c, "", now()))
		if i == saved/2 {
			l = study.Change(l, study.Save("gone1", "gone", "Gone", c, "", now()))
		}
	}
	if data, _ := json.Marshal(l.Pending); len(data) <= sync.MaxCall {
		t.Fatalf("the lines saved away take %d bytes, which one call carries", len(data))
	}
	if err := store.Open(dir).SaveLearning(desktop, "lib", l); err != nil {
		t.Fatal(err)
	}
	p, _ := openStudyIn(t, d, dir)
	p.app.WaitUntil(wait, func() bool {
		l, _ := store.Open(dir).Learning(desktop, "lib")
		return len(l.Pending) == 0 && len(l.Records) == saved
	})
	lines := d.Records("lib", sync.KindLine)
	if len(lines) != saved || slices.ContainsFunc(lines, func(r sync.Record) bool { return r.Film == "gone" }) {
		t.Fatalf("the computer has %d lines", len(lines))
	}
	if d.Puts() < 3 {
		t.Fatalf("the lines went in %d puts", d.Puts())
	}
}

// A note being written stays with the line it was begun on while the film plays on to the next, and is saved to that line; saved empty, it clears the line's note.
func TestANoteBeingWrittenStaysWithItsLineAndCanBeCleared(t *testing.T) {
	d := newDesktop(t)
	p, _ := studyPlayer(t, d)
	p.app.Press("id:player-tab-notes")
	p.app.Type("id:player-note", "a greeting")
	p.at(3200 * time.Millisecond)
	if got := p.app.Find("id:player-note").Text; got != "a greeting" {
		t.Fatalf("as the next line plays the note reads %q", got)
	}
	if got := p.app.Find("id:player-note").Props.Label; got != msgs.PlayerNoteFor(1) {
		t.Fatalf("as the next line plays the note is %q", got)
	}
	p.app.Press("id:player-note-save")
	line := func() (sync.Record, bool) {
		for _, r := range d.Records("lib", sync.KindLine) {
			if r.Removed == nil {
				return r, true
			}
		}
		return sync.Record{}, false
	}
	p.app.WaitUntil(wait, func() bool { r, ok := line(); return ok && fieldText(r, sync.FieldNote) == "a greeting" })
	if r, _ := line(); string(r.Fields[sync.FieldStartMS].Value) != "1000" {
		t.Fatalf("the note went to the line at %s", r.Fields[sync.FieldStartMS].Value)
	}
	// Saved, the field shows the line the card shows again.
	if got := p.app.Find("id:player-note").Props.Label; got != msgs.PlayerNoteFor(2) {
		t.Fatalf("once saved the note is %q", got)
	}

	p.at(1500 * time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:player-note").Text == "a greeting" })
	p.app.Clear("id:player-note")
	p.app.Press("id:player-note-save")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerNoteCleared(), wait)
	p.app.WaitUntil(wait, func() bool { r, ok := line(); return ok && fieldText(r, sync.FieldNote) == "" })
}
