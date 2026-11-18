package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/lookup"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/apps/kando/study"
	"kando/packages/media"
	recordstudy "kando/packages/study"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// vocabTalk is the film "talk" opened and loaded, 12 seconds long, with the AI service of tools, held at 0:01.5 on its first line, "Hello." under its translation "你好。".
func vocabTalk(t *testing.T, tools media.Tools) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, 12, tools)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	return a, tt, updates
}

// vocabTalkQuery is what word, clicked in line i of the open film, is looked up with.
func vocabTalkQuery(a *app, word string, i int) (q lookup.Query) {
	onEngine(a, func() {
		item, _ := a.openFilm()
		_, line := film.SplitCue(a.playerCues()[i].Text)
		q = vocabQuery(item, a.prefs.language, word, line)
	})
	return q
}

// TestLookUpAWordOnThePicture clicks a word of the original line on the picture while the film plays: the film holds and the card over it says the AI service is asked, then shows the word as a dictionary lists it, its reading and what it means, with the line; the AI service was asked about the word in its line, in the film's language. Add to Vocabulary puts the word in the library's study record with the line, its translation and where it is, and the picture says so; Escape closes the card and leaves the film open. Looking the word up again asks the AI service nothing, and In Vocabulary takes it out again.
func TestLookUpAWordOnThePicture(t *testing.T) {
	srv, seen := fakeLLM(t, `{"word":"hello","reading":"/həˈləʊ/","meaning":"int. 你好（打招呼）"}`)
	a, tt, updates := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	a.saved.now = func() time.Time { return at }
	msgs := testlocale.Messages()
	tt.Key("space")
	if !a.player.play.Playing() {
		t.Fatal("Space did not play the film")
	}
	tt.Press(`id:film-subtitle-lines >> text:"Hello"`)
	if a.player.play.Playing() || !a.vocab.lookup.open || !a.vocab.lookup.busy {
		t.Fatalf("after the click: playing %v, lookup %+v", a.player.play.Playing(), a.vocab.lookup)
	}
	wantText(t, tt, msgs.VocabLookupBusy())
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	for _, want := range []string{"hello", "/həˈləʊ/", "int. 你好（打招呼）", "Hello.", msgs.VocabAddToVocabulary(), msgs.FilmExplainLine()} {
		wantText(t, tt, want)
	}
	sent := strings.Join(*seen, "\n")
	for _, want := range []string{"字幕：Hello.", "要查的词：Hello", "用简体中文解释这个词"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}

	tt.Press("id:film-lookup-save")
	book, err := saved.Load(a.snap().LibraryDir)
	if err != nil || len(book.Words) != 1 {
		t.Fatalf("the study record holds %+v, %v", book.Words, err)
	}
	w := book.Words[0]
	if w.Text != "hello" || w.Reading != "/həˈləʊ/" || w.Meaning != "int. 你好（打招呼）" || w.Language != "简体中文" || w.Context != "Hello." || w.ContextTranslation != "你好。" ||
		w.Video != "talk/talk.mp4" || w.Start != time.Second || w.Film != "talk" || !w.SavedAt.Equal(at) || w.Device != a.saved.device || a.saved.device == "" {
		t.Fatalf("the word is %+v", w)
	}
	if a.player.film.osd != msgs.VocabOsdAdded("hello") {
		t.Fatalf("the picture says %q", a.player.film.osd)
	}
	wantText(t, tt, msgs.VocabInVocabulary())

	tt.Key("escape")
	if a.vocab.lookup.open || a.openKey == "" || tt.Exists("id:film-lookup") {
		t.Fatalf("Escape: lookup %v, open film %q", a.vocab.lookup.open, a.openKey)
	}

	asked := len(*seen)
	tt.Press(`id:film-subtitle-lines >> text:"Hello"`)
	if a.vocab.lookup.busy || len(*seen) != asked {
		t.Fatalf("the word looked up again asked the AI service: busy %v", a.vocab.lookup.busy)
	}
	wantText(t, tt, "int. 你好（打招呼）")
	wantText(t, tt, msgs.VocabInVocabulary())
	tt.Press("id:film-lookup-save")
	if book, _ := saved.Load(a.snap().LibraryDir); len(book.Words) != 0 || len(book.RemovedWords) != 1 || a.player.film.osd != msgs.VocabOsdRemoved("hello") {
		t.Fatalf("In Vocabulary left %+v; the picture says %q", book.Words, a.player.film.osd)
	}
	tt.Press("id:film-lookup-close")
	if a.vocab.lookup.open {
		t.Fatal("× left the card open")
	}
}

// TestLookupsAreKeptOnThisComputer checks that a word looked up is kept beside settings.json, so a new start of the app finds it without asking.
func TestLookupsAreKeptOnThisComputer(t *testing.T) {
	srv, seen := fakeLLM(t, `{"word":"bye","reading":"/baɪ/","meaning":"再见"}`)
	a, tt, updates := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	apply(tt, a, func() { a.LookUpWord(1, "Bye") })
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	if len(*seen) == 0 {
		t.Fatal("the AI service was not asked")
	}
	e, ok := lookup.Load(filepath.Join(filepath.Dir(a.settingsPath), lookup.FileName)).Get(vocabTalkQuery(a, "Bye", 1))
	if !ok || e.Meaning != "再见" || e.Word != "bye" {
		t.Fatalf("the cache on disk holds %+v, %v", e, ok)
	}
}

// TestLookupOfflineAsksForALocalAIService checks the card while offline mode is on and the AI service is in the cloud: the word is not sent, the card says only a local AI service can look it up, and Open Settings opens AI Service; a word looked up before still shows offline.
func TestLookupOfflineAsksForALocalAIService(t *testing.T) {
	_, seen := fakeLLM(t, `{"word":"x","meaning":"x"}`)
	msgs := testlocale.Messages()
	a, tt, _ := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", APIKey: "k", Model: "m1", Offline: true}})
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	if len(*seen) != 0 || !a.vocab.lookup.failed || !a.vocab.lookup.needsAI {
		t.Fatalf("offline: sent %q, lookup %+v", *seen, a.vocab.lookup)
	}
	wantText(t, tt, msgs.PrivacyNoCloudAI())
	if tt.Exists("id:film-lookup-save") || tt.Exists("id:film-lookup-explain") {
		t.Fatal("the card offers to add or explain a word it has no meaning for")
	}
	tt.Press("id:film-lookup-settings")
	wantSettings(t, a, settings.SectionAI)
	if a.vocab.lookup.open {
		t.Fatal("Open Settings left the card open")
	}

	a.vocabCache().Put(vocabTalkQuery(a, "Bye", 1), lookup.Entry{Word: "bye", Meaning: "再见"}, time.Now())
	apply(tt, a, func() { a.LookUpWord(1, "Bye") })
	if a.vocab.lookup.failed || a.vocab.lookup.entry.Meaning != "再见" {
		t.Fatalf("a word looked up before, offline: %+v", a.vocab.lookup)
	}
}

// TestLookupWithoutAIServiceSaysSo checks the card when no AI service is set up: Kando has no dictionary of its own, so the card says a lookup needs one and offers its settings.
func TestLookupWithoutAIServiceSaysSo(t *testing.T) {
	a, tt, _ := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	wantText(t, tt, testlocale.Messages().VocabLookupNoAI())
	if !tt.Exists("id:film-lookup-settings") {
		t.Fatal("no Open Settings")
	}
}

// TestWatchTimeGoesIntoTheStudyRecord plays the film for a while, the player reporting every two seconds, and holds it: the time it played goes into the library's study record on this computer's entry for the day, a minute at a time while it plays and the rest as it holds; a gap between two reports longer than a few seconds, as while the computer slept, does not count.
func TestWatchTimeGoesIntoTheStudyRecord(t *testing.T) {
	a, tt, _ := playerOpen(t)
	clock := time.Date(2026, 11, 18, 9, 0, 0, 0, time.Local)
	a.saved.now = func() time.Time { return clock }
	tt.Key("space")
	at := 1500 * time.Millisecond
	step := func(wall, film time.Duration) {
		clock = clock.Add(wall)
		at += film
		playerPlayTo(tt, a, at)
	}
	for range 5 {
		step(2*time.Second, 100*time.Millisecond)
	}
	step(10*time.Minute, 100*time.Millisecond)
	for range 30 {
		step(2*time.Second, 100*time.Millisecond)
	}
	watched := func() time.Duration {
		book, _ := saved.Load(a.snap().LibraryDir)
		var total time.Duration
		for _, w := range book.Watched {
			if w.Day != "2026-11-18" || w.Device != a.saved.device {
				t.Fatalf("watched %+v", book.Watched)
			}
			total += w.Time
		}
		return total
	}
	if got := watched(); got != time.Minute {
		t.Fatalf("after 70 seconds playing the record holds %v, want the first minute", got)
	}
	tt.Key("space")
	if got := watched(); got != 70*time.Second {
		t.Fatalf("held, the record holds %v, want 70s", got)
	}
}

// TestStatsDialogShowsTheStudy opens Study › Study Stats over a library whose study record has words, lines and days watched, and checks its numbers; then its Export Vocabulary tab, whose preview follows the format and the options, and whose Export… writes the file in the folder picked and says where.
func TestStatsDialogShowsTheStudy(t *testing.T) {
	a, tt, _ := playerOpen(t)
	msgs := testlocale.Messages()
	now := time.Date(2026, 11, 18, 21, 0, 0, 0, time.Local)
	a.saved.now = func() time.Time { return now }
	a.savedSync()
	book := &a.saved.book
	for d, m := range map[int]int{16: 30, 17: 20, 18: 40, 10: 10} {
		book.MergeWatched([]recordstudy.Watched{{Day: recordstudy.Day(time.Date(2026, 11, d, 12, 0, 0, 0, time.Local)), Device: a.saved.device, Time: time.Duration(m) * time.Minute}})
	}
	book.MergeWatched([]recordstudy.Watched{{Day: "2026-11-18", Device: "phone", Time: 15 * time.Minute}})
	book.AddWord(saved.Word{Text: "trickle", Reading: "/ˈtrɪk(ə)l/", Meaning: "细流", Context: "Every river begins as a trickle.", Film: "Rivers"})
	book.AddWord(saved.Word{Text: "meander", Meaning: "河曲", Film: "Rivers"})
	book.Toggle(saved.Line{Video: "talk/talk.mp4", Start: time.Second, Original: "Hello.", Translation: "你好。", Film: "talk", SavedAt: now})
	a.savedWrite()

	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), groupTitle(a.cx, groupStudy), msgs.VocabStatsMenuItem()))
	if !a.vocab.stats.open || !a.modalOpen() {
		t.Fatal("Study Stats did not open the dialog")
	}
	// The last seven days hold 105 minutes, 15 of them on the phone; the seven before, 10.
	for _, want := range []string{msgs.VocabDays(3), msgs.VocabLongest(3), msgs.VocabHours("1.8"), msgs.VocabWeekMore(msgs.VocabHours("1.6")),
		msgs.VocabWordCount(2), msgs.VocabWordsDue(2), msgs.VocabLineCount(1), msgs.VocabLinesDue(1), msgs.VocabNew(3), msgs.VocabMastered(0)} {
		wantText(t, tt, want)
	}

	tt.Press("id:stats-tab-export")
	wantText(t, tt, msgs.VocabSummary(2, 0, 1))
	if p := tt.Find("id:stats-preview").Text; !strings.HasPrefix(p, `"meander"`) || !strings.Contains(p, `"trickle"`) {
		t.Fatalf("the Anki preview reads %q", p)
	}
	tt.Press("id:stats-format-csv")
	tt.Press("id:stats-with-lines")
	wantText(t, tt, msgs.VocabSummary(2, 1, 2))
	if p := tt.Find("id:stats-preview").Text; !strings.HasPrefix(p, "meander,,河曲,,,Rivers") || !strings.Contains(p, "Hello.,,你好。,,,talk") {
		t.Fatalf("the CSV preview reads %q", p)
	}
	out := t.TempDir()
	tt.PickDirectories = func(ns.DirectoriesRequest) []ns.Directory { return []ns.Directory{{Path: out}} }
	tt.Press("id:stats-export")
	data, err := os.ReadFile(filepath.Join(out, "Kando Vocabulary.csv"))
	if err != nil || !strings.Contains(string(data), "Word,Reading,Meaning,Sentence,Translation,Film") || !strings.Contains(string(data), "trickle") {
		t.Fatalf("wrote %q, %v", data, err)
	}
	if e := a.vocab.stats.exported; e.File != filepath.Join(out, "Kando Vocabulary.csv") || e.Err != "" {
		t.Fatalf("exported %+v", e)
	}
	tt.Press("id:stats-done")
	if a.vocab.stats.open || a.modalOpen() {
		t.Fatal("Done left the dialog open")
	}
	if a.vocab.stats.tab != study.TabExport {
		t.Fatal("the dialog forgot its tab")
	}
}

// TestWatchTimeLeavesAnUnreadableStudyRecordAlone plays the film over a study record a later Kando wrote, which this one cannot read: the time played is not written into it, the file stays as it was, and the picture does not say the saved lines could not be saved, as the learner saved nothing.
func TestWatchTimeLeavesAnUnreadableStudyRecordAlone(t *testing.T) {
	a, tt, _ := playerOpen(t)
	path := filepath.Join(a.snap().LibraryDir, recordstudy.FileName)
	later := []byte(`{"version": 99, "lines": []}`)
	if err := os.WriteFile(path, later, 0o644); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 18, 9, 0, 0, 0, time.Local)
	a.saved.now = func() time.Time { return clock }
	tt.Key("space")
	at := 1500 * time.Millisecond
	for range 35 {
		clock = clock.Add(2 * time.Second)
		at += 100 * time.Millisecond
		playerPlayTo(tt, a, at)
	}
	tt.Key("space")
	failed := testlocale.Messages().SavedWriteFailed()
	if a.player.film.osd == failed {
		t.Fatalf("the picture says %q", failed)
	}
	if data, _ := os.ReadFile(path); string(data) != string(later) {
		t.Fatalf("the study record was written over: %s", data)
	}
}

// TestLookupTellsWordsOfTwoLanguagesApart looks up "Hello" in the film while the vocabulary holds a "hello" of a film spoken in another language: the card offers Add to Vocabulary, and pressing it adds the word of this film beside the other rather than taking the other out.
func TestLookupTellsWordsOfTwoLanguagesApart(t *testing.T) {
	a, tt, _ := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	msgs := testlocale.Messages()
	q := vocabTalkQuery(a, "Hello", 0)
	other := "xx"
	if q.Spoken == other {
		t.Fatalf("the film is spoken in %q", other)
	}
	onEngine(a, func() {
		a.savedSync()
		a.saved.book.AddWord(saved.Word{Text: "hello", Spoken: other, Meaning: "another language's hello"})
		a.savedWrite()
		a.vocabCache().Put(q, lookup.Entry{Word: "hello", Meaning: "你好"}, time.Now())
	})
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	wantText(t, tt, msgs.VocabAddToVocabulary())
	tt.Press("id:film-lookup-save")
	book, _ := saved.Load(a.snap().LibraryDir)
	if len(book.Words) != 2 || book.WordIndex("hello", other) < 0 || book.WordIndex("hello", q.Spoken) < 0 || len(book.RemovedWords) != 0 {
		t.Fatalf("the vocabulary holds %+v, removed %+v", book.Words, book.RemovedWords)
	}
	wantText(t, tt, msgs.VocabInVocabulary())
}
