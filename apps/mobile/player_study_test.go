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

// The word card says where the meaning came from, as the computer says it: a dictionary installed there, with the credit its license asks for, a plugin's lookup source there, or the AI service of the computer that answered, by the name it sent with the answer, or without a name when it sent none; an older computer that does not say leaves it out.
func TestTheWordCardSaysWhereTheMeaningCameFrom(t *testing.T) {
	d := newDesktop(t)
	d.SetEntry("Good", sync.WordEntry{Word: "good", Meaning: "adj. 好", Language: "简体中文", Source: sync.WordSource{Kind: sync.SourceDictionary, Name: "CC-CEDICT", Credit: "CC-CEDICT © MDBG"}})
	d.SetEntry("morning", sync.WordEntry{Word: "morning", Meaning: "n. 早晨", Language: "简体中文", Source: sync.WordSource{Kind: sync.SourceAI, Desktop: "Office"}})
	p, _ := studyPlayer(t, d)
	source := func(word string) (string, bool) {
		p.app.Press(`id:player-card >> text:"` + word + `"`)
		p.app.WaitFor("id:player-word-meaning", wait)
		defer p.app.Press("id:player-word-close")
		if !p.app.Exists("id:player-word-source") {
			return "", false
		}
		return p.app.Find("id:player-word-source").Text, true
	}
	if got, _ := source("Good"); got != msgs.PlayerWordSourceDictCredit("CC-CEDICT", "CC-CEDICT © MDBG") {
		t.Fatalf("a dictionary's meaning says %q", got)
	}
	// The name the computer sent with the answer, not that of the film's computer.
	if got, _ := source("morning"); got != msgs.PlayerWordSourceAI("Office") {
		t.Fatalf("the AI service's meaning says %q", got)
	}

	older := newDesktop(t)
	older.SetEntry("morning", sync.WordEntry{Word: "morning", Meaning: "n. 早晨", Language: "简体中文"})
	older.SetEntry("Good", sync.WordEntry{Word: "good", Meaning: "adj. 好", Language: "简体中文", Source: sync.WordSource{Kind: sync.SourceAI}})
	p, _ = studyPlayer(t, older)
	if got, _ := source("Good"); got != msgs.PlayerWordSourceAIUnnamed() {
		t.Fatalf("the AI service's meaning without a name says %q", got)
	}
	if got, ok := source("morning"); ok {
		t.Fatalf("an older computer's meaning says %q", got)
	}

	// A plugin's lookup source on the computer, by its name and its plugin's, not as a dictionary.
	plugin := newDesktop(t)
	plugin.SetEntry("Good", sync.WordEntry{Word: "good", Meaning: "adj. 好", Language: "简体中文", Source: sync.WordSource{Kind: sync.SourcePlugin, Name: "河流词典", Plugin: "河流"}})
	p, _ = studyPlayer(t, plugin)
	if got, _ := source("Good"); got != msgs.PlayerWordSourcePlugin("河流", "河流词典") {
		t.Fatalf("a plugin's meaning says %q", got)
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
	// A computer that does not say its model runs on it is taken to ask a cloud service.
	if got := p.app.Find("id:player-explain-by").Text; got != msgs.PlayerExplainVia("test-model", "Studio") {
		t.Fatalf("the byline reads %q", got)
	}
	x := d.Explains()
	if len(x) != 1 || x[0].Index != 1 || len(x[0].Lines) != 3 || x[0].Lines[1].Original != "How are you?" || x[0].Lines[1].StartMS != 3000 {
		t.Fatalf("the computer was asked %+v", x)
	}
}

// The explanation shows the computer's sections under their headings, its tone and related expressions after More; a follow-up question goes to the computer with the line, the lines around it and what was explained and answered before, and its answer shows under it; Save Line to Notes saves the line with what it means as its note.
func TestExplainShowsSectionsAnswersFollowUpsAndSavesToNotes(t *testing.T) {
	d := newDesktop(t)
	d.SetExplanationSections(
		sync.ExplainSection{Kind: sync.ExplainMeaning, Text: "Asks how someone is."},
		sync.ExplainSection{Kind: sync.ExplainBreakdown, Text: "how · 怎样"},
		sync.ExplainSection{Kind: sync.ExplainTone, Text: "Friendly."},
	)
	p, _ := studyPlayer(t, d)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-note", wait)
	got := p.texts("id:player-explain-sections")
	for _, want := range []string{msgs.PlayerExplainMeaning(), "Asks how someone is.", msgs.PlayerExplainBreakdown(), "how · 怎样"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the sections read %q, want %q in them", got, want)
		}
	}
	if strings.Contains(got, "Friendly.") || p.app.Exists("id:player-explain-text") {
		t.Fatalf("before More the sheet reads %q", p.texts("id:player-explain"))
	}
	if got := p.app.Find("id:player-explain-note").Text; got != msgs.PlayerExplainOnlyBefore() {
		t.Fatalf("the footnote reads %q", got)
	}
	p.app.Press("id:player-explain-more")
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:player-explain-sections"), "Friendly.") })
	if !strings.Contains(p.texts("id:player-explain-sections"), msgs.PlayerExplainTone()) || p.app.Exists("id:player-explain-more") {
		t.Fatalf("after More the sections read %q", p.texts("id:player-explain-sections"))
	}

	p.app.Type("id:player-explain-ask", "Why how?")
	p.app.Press("id:player-explain-send")
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:player-explain-answer") && p.app.Find("id:player-explain-answer").Text == "Answered: Why how?"
	})
	if got := p.app.Find("id:player-explain-ask").Text; got != "" {
		t.Fatalf("after asking the field reads %q", got)
	}
	p.app.Type("id:player-explain-ask", "And the reply?")
	p.app.Submit("id:player-explain-ask")
	p.app.WaitUntil(wait, func() bool { return len(d.Asks()) == 2 })
	asks := d.Asks()
	first, second := asks[0], asks[1]
	if first.Question != "Why how?" || first.Index != 1 || first.Lines[1].Original != "How are you?" || first.Film != "f1" || first.Library != "lib" ||
		len(first.Explained) != 3 || first.Explained[0].Text != "Asks how someone is." || len(first.Asked) != 0 {
		t.Fatalf("the computer was asked %+v", first)
	}
	if second.Question != "And the reply?" || len(second.Asked) != 1 || second.Asked[0] != (sync.ExplainQA{Question: "Why how?", Answer: "Answered: Why how?"}) {
		t.Fatalf("the second question carried %+v", second)
	}

	p.app.Press("id:player-explain-save")
	p.app.WaitUntil(wait, func() bool {
		for _, r := range d.Records("lib", sync.KindLine) {
			if r.Removed == nil && string(r.Fields[sync.FieldStartMS].Value) == "3000" {
				return fieldText(r, sync.FieldNote) == "Asks how someone is."
			}
		}
		return false
	})
	if got := p.app.Find("id:player-explain-save").Props.Label; got != msgs.PlayerExplainSaved() {
		t.Fatalf("once saved the button reads %q", got)
	}
}

// Without an AI service on the computer, a follow-up question says so under it.
func TestAFollowUpWithoutAnAIServiceSaysWhere(t *testing.T) {
	d := newDesktop(t)
	p, _ := studyPlayer(t, d)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-text", wait)
	d.SetNoAI(true)
	p.app.Type("id:player-explain-ask", "Why?")
	p.app.Press("id:player-explain-send")
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:player-explain-answer") && p.app.Find("id:player-explain-answer").Text == msgs.PlayerStudyNoAI("Studio")
	})
}

// openStudyAt opens the talk from its page, as openStudy does, left at at on the phone before: the player holds there.
func openStudyAt(t *testing.T, d *synctest.Desktop, at time.Duration) *phone {
	t.Helper()
	dir := t.TempDir()
	f := keepTalkFrom(t, dir, store.DesktopID(d.Fingerprint))
	f.PositionMS = at.Milliseconds()
	if err := store.Open(dir).SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	return p
}

// A film opened from its page where it was left holds there and shows the recap of the part watched: the computer is sent the lines up to where it was left, never a later one, and its moments show, each playing the film from its line; Continue plays on from where it was left.
func TestAFilmLeftPartwayOpensWithARecapOfWhatWasWatched(t *testing.T) {
	d := newDesktop(t)
	p := openStudyAt(t, d, 6*sec)
	p.app.WaitFor("id:player-recap-note", wait)
	if !p.app.Find("id:player-video").Video.Paused {
		t.Fatal("the film plays under the recap")
	}
	if got := p.app.Find("id:player-recap-at").Text; got != msgs.PlayerRecapStoppedAt("0:06") {
		t.Fatalf("the recap says %q", got)
	}
	recaps := d.Recaps()
	if len(recaps) != 1 || len(recaps[0].Lines) != 3 || recaps[0].Lines[2].Original != "Fine, thanks." || recaps[0].Film != "f1" {
		t.Fatalf("the computer was asked %+v", recaps)
	}
	if got := p.texts("id:player-recap-items"); got != "0:01\nBegan: Good morning.\n0:05\nThen: Fine, thanks." {
		t.Fatalf("the recap reads %q", got)
	}
	if got := p.app.Find("id:player-recap-note").Text; got != msgs.PlayerRecapNote("test-model") {
		t.Fatalf("the footnote reads %q", got)
	}
	p.app.Press("id:player-recap-continue")
	p.app.WaitUntil(wait, func() bool { return !p.app.Exists("id:player-recap") })
	if s := p.seeks(); len(s) == 0 || s[len(s)-1] != 6*sec || p.app.Find("id:player-video").Video.Paused {
		t.Fatalf("continue went to %v", s)
	}
}

// A moment of the recap plays the film from its line, with the film's subtitle delay; the recap closed with × leaves the film held.
func TestARecapMomentPlaysFromItsLine(t *testing.T) {
	d := newDesktop(t)
	d.SetRecap(sync.RecapItem{StartMS: 3000, Text: "They greet."})
	p := openStudyAt(t, d, 6*sec)
	p.app.WaitFor("id:player-recap-item", wait)
	p.app.Press("id:player-recap-item")
	p.app.WaitUntil(wait, func() bool { return !p.app.Exists("id:player-recap") })
	if s := p.seeks(); len(s) == 0 || s[len(s)-1] != 3*sec || p.app.Find("id:player-video").Video.Paused {
		t.Fatalf("the moment went to %v", s)
	}

	p = openStudyAt(t, newDesktop(t), 6*sec)
	p.app.WaitFor("id:player-recap-note", wait)
	p.app.Press("id:player-recap-close")
	p.app.WaitUntil(wait, func() bool { return !p.app.Exists("id:player-recap") })
	if !p.app.Find("id:player-video").Video.Paused {
		t.Fatal("closing the recap played the film")
	}
}

// Without an AI service on the computer the recap shows the lines watched, saying they are not summed up and why; Start Over plays from the start.
func TestARecapWithoutAnAIServiceShowsTheLinesWatched(t *testing.T) {
	d := newDesktop(t)
	d.SetNoAI(true)
	p := openStudyAt(t, d, 6*sec)
	p.app.WaitFor("id:player-recap-note", wait)
	if got := p.app.Find("id:player-recap-note").Text; got != msgs.PlayerRecapExcerptsNoAI("Studio") {
		t.Fatalf("the footnote reads %q", got)
	}
	if got := p.texts("id:player-recap-items"); got != "0:01\n早上好。\n0:03\n你好吗？\n0:05\nFine, thanks." {
		t.Fatalf("the recap reads %q", got)
	}
	p.app.Press("id:player-recap-start-over")
	p.app.WaitUntil(wait, func() bool { return !p.app.Exists("id:player-recap") })
	if s := p.seeks(); len(s) == 0 || s[len(s)-1] != 0 || p.app.Find("id:player-video").Video.Paused {
		t.Fatalf("start over went to %v", s)
	}
}

// A film left before its first line plays at once, without a recap: nothing of it was watched.
func TestAFilmLeftBeforeItsFirstLinePlaysWithoutARecap(t *testing.T) {
	d := newDesktop(t)
	p := openStudyAt(t, d, 500*time.Millisecond)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") && !p.app.Find("id:player-video").Video.Paused })
	if p.app.Exists("id:player-recap") || len(d.Recaps()) != 0 {
		t.Fatal("a film nothing of which was watched opened with a recap")
	}
}

// fieldText is field name of r as a string.
func fieldText(r sync.Record, name string) string {
	var s string
	_ = json.Unmarshal(r.Fields[name].Value, &s)
	return s
}

// japaneseLine is the first line of the talk as openJapanese has it, a line of Japanese, and japaneseWord a word of it.
const japaneseLine, japaneseWord = "今日は学校に行った。", "学校"

// openJapanese opens the talk in the player as openStudy does, its first line in Japanese, from a desktop that knows the word japaneseWord, once the card has the words of that line to tap: the dictionary that cuts Japanese is read in the background.
func openJapanese(t *testing.T) (*phone, *synctest.Desktop) {
	t.Helper()
	return openJapaneseMarked(t, nil)
}

// openJapaneseMarked is openJapanese with the computer marking the words of the film's lines with marks, none for nil.
func openJapaneseMarked(t *testing.T, marks *sync.FilmMarks) (*phone, *synctest.Desktop) {
	t.Helper()
	lines := slices.Clone(talkCues)
	lines[0].Original = japaneseLine
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese Class")
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Poster: []byte("jpeg"), Cues: lines,
		Film: sync.Film{ID: "f1", Title: "Morning Talk", Source: "youtube", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: now()}})
	d.Offer("123456")
	d.SetEntry(japaneseWord, sync.WordEntry{Word: japaneseWord, Reading: "がっこう", Meaning: "n. 学校", Language: "简体中文"})
	if marks != nil {
		d.SetMarks("f1", *marks)
	}
	dir := t.TempDir()
	f := keepTalkFrom(t, dir, store.DesktopID(d.Fingerprint))
	if err := store.Open(dir).SaveCues(f.Desktop, f.Film.ID, lines); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(time.Minute, func() bool { return p.app.Exists(`id:player-card >> text:"` + japaneseWord + `"`) })
	return p, d
}

// A line of a film in Japanese, written without spaces between words, has words to tap on the card once the dictionary that cuts it is read in the background, and a word tapped is looked up through the computer in its line.
func TestTheWordsOfAJapaneseLineAreTappedOnTheCard(t *testing.T) {
	p, d := openJapanese(t)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(1500 * time.Millisecond)
	p.app.Press(`id:player-card >> text:"` + japaneseWord + `"`)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-word-meaning") })
	if got := d.Lookups(); len(got) != 1 || got[0].Word != japaneseWord || got[0].Line != japaneseLine {
		t.Fatalf("the computer was asked %+v", got)
	}
	if p.app.Find("id:player-word-meaning").Text != "n. 学校" {
		t.Fatalf("the word card reads %q", p.texts("id:player-word-sheet"))
	}
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

// A line saved in the player is among the Learn tab's saved lines at once, away from the computer too, as the two read one learning state; the computer gets it once, when it is back.
func TestALineSavedInThePlayerShowsInLearn(t *testing.T) {
	d := newDesktop(t)
	p, _ := studyPlayer(t, d)
	d.SetOff(true)
	p.app.Press("id:player-save")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerSavedToast(), wait)
	p.app.Press("id:player-minimize")
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:learn-lines") && strings.Contains(p.texts("id:learn-lines"), "Good morning.")
	})

	d.SetOff(false)
	p.learnRefresh()
	lines := func() []sync.Record {
		return slices.DeleteFunc(d.Records("lib", sync.KindLine), func(r sync.Record) bool { return r.Removed != nil })
	}
	p.app.WaitUntil(wait, func() bool { return len(lines()) > 0 })
	if got := lines(); len(got) != 1 || fieldText(got[0], sync.FieldOriginal) != "Good morning." {
		t.Fatalf("the computer has %+v, want the line saved once", got)
	}
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
	movingClock(t)
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

// words are the computer's word records of library "lib" not removed.
func words(d *synctest.Desktop) []sync.Record {
	return slices.DeleteFunc(d.Records("lib", sync.KindWord), func(r sync.Record) bool { return r.Removed != nil })
}

// A word looked up goes in the vocabulary from its card, with the line it was looked up in, and the computer gets it; the Words tab lists it with how many times the film says it, and its star takes it out again, on the computer too.
func TestAWordLookedUpGoesInTheVocabularyFromItsCardAndTheWordsTab(t *testing.T) {
	d := newDesktop(t)
	d.SetEntry("morning", sync.WordEntry{Word: "morning", Reading: "/ˈmɔːnɪŋ/", Meaning: "n. 早晨", Language: "简体中文"})
	p, _ := studyPlayer(t, d)
	p.app.Press("id:player-tab-words")
	p.app.WaitFor("id:player-words-empty", wait)

	p.app.Press(`id:player-card >> text:"morning"`)
	p.app.WaitFor("id:player-word-vocab", wait)
	if !p.app.Exists("id:player-word-vocab >> text:" + msgs.PlayerWordAdd()) {
		t.Fatalf("the word card offers %q", p.texts("id:player-word-vocab"))
	}
	p.app.Press("id:player-word-vocab")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerWordAdded("morning"), wait)
	p.app.WaitUntil(wait, func() bool { return len(words(d)) == 1 })
	w := words(d)[0]
	if w.Film != "f1" || fieldText(w, sync.FieldWord) != "morning" || fieldText(w, sync.FieldMeaning) != "n. 早晨" || fieldText(w, sync.FieldContext) != "Good morning." ||
		fieldText(w, sync.FieldContextTranslation) != "早上好。" || fieldText(w, sync.FieldSpoken) != "ja" || fieldText(w, sync.FieldTitle) != "Morning Talk" {
		t.Fatalf("the computer has %+v", w)
	}
	// The word's line keeps its end, so its sound is cut to the line's real length.
	var start, end int64
	_ = json.Unmarshal(w.Fields[sync.FieldStartMS].Value, &start)
	_ = json.Unmarshal(w.Fields[sync.FieldEndMS].Value, &end)
	if start != 1000 || end != 2000 {
		t.Fatalf("the word's line is at %d–%d ms", start, end)
	}
	if !p.app.Exists("id:player-word-vocab >> text:" + msgs.PlayerWordInVocabulary()) {
		t.Fatalf("once added the word card offers %q", p.texts("id:player-word-vocab"))
	}
	p.app.Press("id:player-word-close")

	if got := p.texts("id:player-words"); !strings.Contains(got, "morning\n/ˈmɔːnɪŋ/\nn. 早晨\n"+msgs.PlayerWordTimes(1)) {
		t.Fatalf("the Words tab reads %q", got)
	}
	if !p.app.Exists("id:player-tab-words >> text:" + msgs.PlayerTabWordsCount(1)) {
		t.Fatal("the Words tab does not count the word")
	}
	p.app.Press(`label:"` + msgs.PlayerWordRemoveNamed("morning") + `"`)
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerWordRemoved("morning"), wait)
	p.app.WaitUntil(wait, func() bool { return len(words(d)) == 0 && len(d.Records("lib", sync.KindWord)) == 1 })
	// Looked up, it stays in the tab, to add again.
	if !p.app.Exists(`label:"` + msgs.PlayerWordAddNamed("morning") + `"`) {
		t.Fatalf("the Words tab reads %q", p.texts("id:player-words"))
	}
}

// A word the computer has in the vocabulary from the film shows in the Words tab, starred; a tap shows its card.
func TestTheComputersWordsOfTheFilmShowInTheWordsTab(t *testing.T) {
	d := newDesktop(t)
	at := now().Add(-time.Hour)
	field := func(v any) sync.Field { return sync.Field{Value: sync.Value(v)} }
	d.SetState("lib", at, sync.Record{Kind: sync.KindWord, ID: "deskword", Film: "f1", Fields: map[string]sync.Field{
		sync.FieldWord: field("see"), sync.FieldSpoken: field("ja"), sync.FieldMeaning: field("v. 看见"), sync.FieldContext: field("See you."),
		sync.FieldTitle: field("Morning Talk"), sync.FieldStartMS: field(7000), sync.FieldSavedAt: field(at), sync.FieldReview: field(sync.Review{Due: at}),
	}})
	p, _ := studyPlayer(t, d)
	p.app.Press("id:player-tab-words")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists(`label:"` + msgs.PlayerWordRemoveNamed("see") + `"`) })
	if got := p.texts("id:player-words"); !strings.Contains(got, "see\nv. 看见\n"+msgs.PlayerWordTimes(1)) {
		t.Fatalf("the Words tab reads %q", got)
	}
	p.app.Press("id:player-film-word")
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-word-line") })
	if got := p.app.Find("id:player-word-line").Text; got != "See you." {
		t.Fatalf("the word's card shows the line %q", got)
	}
}

// Minimized, the player leaves a bar over the tabs while the film plays on: it shows the line playing, holds and plays the film, plays the next line, and opens the same player again, where the film is.
func TestTheMiniPlayerPlaysOnOverTheTabs(t *testing.T) {
	p, f := openPlayer(t)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	if p.app.Exists("id:player-card") || p.app.Exists("id:player-tabs") {
		t.Fatal("minimized, the player still shows")
	}
	if !p.app.Exists(talkRow(f.Desktop)) {
		t.Fatal("minimized, the Library does not show under the mini player")
	}
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(p.dir).Films()
		return len(films) == 1 && films[0].PositionMS == 3200
	})
	if p.app.Find("id:player-video").Video.Paused {
		t.Fatal("minimized, the film stopped")
	}
	if got := p.app.Find("id:player-mini-line").Text; got != "你好吗？" {
		t.Fatalf("the mini player shows %q", got)
	}
	if got := p.app.Find("id:player-mini-original").Text; got != "How are you?" {
		t.Fatalf("the mini player's original reads %q", got)
	}
	p.at(5200 * time.Millisecond)
	if got := p.app.Find("id:player-mini-line").Text; got != "Fine, thanks." {
		t.Fatalf("an untranslated line shows %q", got)
	}
	if got := p.app.Find("id:player-mini-original").Text; got != f.Film.Title {
		t.Fatalf("under an untranslated line the mini player shows %q", got)
	}
	p.app.Press("id:player-mini-play")
	if !p.app.Find("id:player-video").Video.Paused {
		t.Fatal("the mini player's button did not hold the film")
	}
	p.app.Press("id:player-mini-next")
	if s := p.seeks(); s[len(s)-1] != 7*sec || p.app.Find("id:player-video").Video.Paused {
		t.Fatalf("next line went to %v", s)
	}
	loads := len(p.seeks())
	p.app.Press("id:player-mini-open")
	p.app.WaitFor("id:player-card", wait)
	if p.app.Exists("id:player-mini") {
		t.Fatal("the mini player shows over the player")
	}
	if len(p.seeks()) != loads || p.app.Find("id:player-video").Video.Paused {
		t.Fatalf("the player opened again went to %v", p.seeks()[loads:])
	}
	// Played again from its row, the film opens the same player.
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-card", wait)
	if len(p.seeks()) != loads {
		t.Fatalf("played from its row, the film went to %v", p.seeks()[loads:])
	}
}

// A film played from a line a search found while it plays in the mini player plays from that line, in the same player.
func TestAFilmPlayedFromALineWhileMinimizedGoesThere(t *testing.T) {
	p, f := openPlayer(t)
	p.at(7500 * time.Millisecond)
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:films-search", wait)
	p.app.Type("id:films-search", "HOW ARE")
	p.app.WaitFor("id:films-hit-"+f.Desktop+"-f1", wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-card", wait)
	if s := p.seeks(); s[len(s)-1] != 3*sec {
		t.Fatalf("the player went to %v", s)
	}
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 3000 })
}

// A film deleted from the phone while it plays in the mini player stops, and the mini player goes. The film's page, where it was downloaded and played, keeps its actions clear of the mini player.
func TestDeletingTheFilmPlayingMinimizedStopsIt(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.openTalk(d)
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-play", wait)
	p.app.Press("id:film-play")
	p.app.WaitFor("id:player-video", wait)
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	if play, mini := p.app.Find("id:film-play").Bounds(), p.app.Find("id:player-mini").Bounds(); play.Origin.Y+play.Size.Height > mini.Origin.Y {
		t.Fatalf("the mini player %+v covers the film's Play button %+v", mini, play)
	}
	p.app.Press("id:film-delete")
	p.app.Answer(1)
	p.app.WaitForGone("id:player-mini", wait)
	if p.app.Exists("id:player-video") {
		t.Fatal("the deleted film still plays")
	}
}

// Back, as Android's button or Escape, closes the word card over the player first, then minimizes the player, and only then goes back in the tab under it, the film staying in the mini player as it was.
func TestBackMinimizesThePlayerBeforeItLeavesThePageUnderIt(t *testing.T) {
	d := newDesktop(t)
	d.SetEntry("morning", sync.WordEntry{Word: "morning", Meaning: "n. 早晨", Language: "简体中文"})
	p, f := studyPlayer(t, d)
	p.app.Press(`id:player-card >> text:"morning"`)
	p.app.WaitFor("id:player-word-vocab", wait)
	p.app.Back()
	p.app.WaitForGone("id:player-word-vocab", wait)
	if !p.app.Exists("id:player-card") || p.app.Exists("id:player-mini") {
		t.Fatal("back closed more than the word card")
	}
	p.app.Back()
	p.app.WaitFor("id:player-mini", wait)
	if p.app.Exists("id:player-card") || !p.app.Exists(talkRow(f.Desktop)) {
		t.Fatal("back did not minimize the player over the Library")
	}
	// A page opened in a tab under the mini player goes back as before.
	paused := p.app.Find("id:player-video").Video.Paused
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-computer")
	p.app.WaitFor("id:computer-page", wait)
	p.app.Back()
	p.app.WaitForGone("id:computer-page", wait)
	if !p.app.Exists("id:player-mini") {
		t.Fatal("going back in the tab closed the mini player")
	}
	if p.app.Find("id:player-video").Video.Paused != paused {
		t.Fatal("going back in the tab played or held the film in the mini player")
	}
}
