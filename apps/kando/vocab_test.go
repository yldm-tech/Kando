package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/lookup"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/study"
	"kando/packages/core"
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
	tt.PickDirectories = func(nsgotest.DirectoryPick) []ns.Directory { return []ns.Directory{{Path: out}} }
	tt.Press("id:stats-export")
	data, err := os.ReadFile(filepath.Join(out, "Kando Vocabulary.csv"))
	if err != nil || !strings.Contains(string(data), "Word,Reading,Meaning,Sentence,Translation,Film") || !strings.Contains(string(data), "trickle") {
		t.Fatalf("wrote %q, %v", data, err)
	}
	if e := a.vocab.stats.exported; e.File != filepath.Join(out, "Kando Vocabulary.csv") || e.Err != "" {
		t.Fatalf("exported %+v", e)
	}
	// Another interface language renames the tabs and the formats in place.
	csv := tt.Find("id:stats-format-csv").ID
	a.cx.SetLocale("en")
	refresh(tt, a)
	en := locales.In("en")
	if got := tt.Find("id:stats-format-csv"); got.ID != csv || got.Props.Label != en.VocabFormatCSV() {
		t.Fatalf("in English the CSV format is view %d (was %d) named %q", got.ID, csv, got.Props.Label)
	}
	if got := tt.Find("id:stats-tab-export").Props.Label; got != en.VocabTabExport() {
		t.Fatalf("in English the export tab is named %q", got)
	}
	if got := tt.Find("id:stats").Props.Label; got != en.VocabStatsTitle() {
		t.Fatalf("in English the dialog is named %q", got)
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

// TestStatsCountTheUnlockedLibraries opens Study › Study Stats with three libraries Kando knows, each with a study record: the open one, another one and a locked one. The stats add up the open and the other one's minutes, streak, words and lines, and name them; the locked one is left out, said to be, and its study record is never read nor written.
func TestStatsCountTheUnlockedLibraries(t *testing.T) {
	parent := t.TempDir()
	open, other, locked := filepath.Join(parent, "甲.kando"), filepath.Join(parent, "乙.kando"), filepath.Join(parent, "丙.kando")
	now := time.Date(2026, 11, 18, 21, 0, 0, 0, time.Local)
	write := func(dir string, days map[int]int, words ...string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		book, err := recordstudy.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		book.Device = "mac"
		book.Now = func() time.Time { return now }
		for d, m := range days {
			book.Watch(time.Date(2026, 11, d, 12, 0, 0, 0, time.Local), time.Duration(m)*time.Minute)
		}
		for _, w := range words {
			book.AddWord(saved.Word{Text: w, Meaning: w})
		}
		if err := book.Save(); err != nil {
			t.Fatal(err)
		}
	}
	write(open, map[int]int{18: 20}, "trickle")
	write(other, map[int]int{17: 10, 16: 5}, "meander", "delta")
	write(locked, map[int]int{15: 30, 14: 30}, "estuary", "silt", "levee")
	lockedRecord, err := os.ReadFile(filepath.Join(locked, recordstudy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	var read []string
	prev := vocabLoadBook
	vocabLoadBook = func(dir string) (saved.Book, error) {
		read = append(read, dir)
		return prev(dir)
	}
	t.Cleanup(func() { vocabLoadBook = prev })

	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.saved.device = "mac"
	a.saved.now = func() time.Time { return now }
	lock, err := shelf.NewLock(lockPassword)
	if err != nil {
		t.Fatal(err)
	}
	a.library.SetLock(locked, lock)
	a.edit(func(s *session) {
		s.LibraryDir = open
		s.Libraries = []string{open, other, locked}
	})
	refresh(tt, a)
	msgs := testlocale.Messages()
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), groupTitle(a.cx, groupStudy), msgs.VocabStatsMenuItem()))
	waitUntil(t, tt, a, updates, func() bool { return len(a.vocab.stats.others) == 1 })

	// 甲 and 乙 together: three days in a row, 35 minutes this week, three words, none from 丙.
	for _, want := range []string{msgs.VocabStatsLibraries("甲" + msgs.VocabLibrarySeparator() + "乙"), msgs.VocabStatsLocked(1),
		msgs.VocabDays(3), msgs.VocabLongest(3), msgs.VocabMinutes(35), msgs.VocabWordCount(3), msgs.VocabWordsDue(3), msgs.VocabNew(3)} {
		wantText(t, tt, want)
	}
	if slices.Contains(read, locked) || slices.Contains(read, open) {
		t.Fatalf("the stats read %q", read)
	}
	if after, err := os.ReadFile(filepath.Join(locked, recordstudy.FileName)); err != nil || !bytes.Equal(after, lockedRecord) {
		t.Fatalf("the locked library's study record changed: %v", err)
	}
	tt.Press("id:stats-done")
	if a.vocab.stats.others != nil {
		t.Fatal("the other libraries' records stayed in memory after Done")
	}
}

// TestStatsLeaveOutALockedOpenLibraryAndMissingOnes opens Study › Study Stats while the open library is locked and another library's folder is gone: neither is counted nor named, and the stats are those of the one library left.
func TestStatsLeaveOutALockedOpenLibraryAndMissingOnes(t *testing.T) {
	parent := t.TempDir()
	open, other, gone := filepath.Join(parent, "甲.kando"), filepath.Join(parent, "乙.kando"), filepath.Join(parent, "丁.kando")
	for _, dir := range []string{open, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		book, err := recordstudy.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		book.Device = "mac"
		book.AddWord(saved.Word{Text: filepath.Base(dir), Meaning: "x"})
		if err := book.Save(); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	lock, err := shelf.NewLock(lockPassword)
	if err != nil {
		t.Fatal(err)
	}
	a.library.SetLock(open, lock)
	a.edit(func(s *session) {
		s.LibraryDir = open
		s.Libraries = []string{open, other, gone}
	})
	refresh(tt, a)
	onEngine(a, func() { a.OpenStats(vocabTabStats) })
	waitUntil(t, tt, a, updates, func() bool { return len(a.vocab.stats.others) == 1 })
	if got := a.StatsLibraries(); !slices.Equal(got, []string{"乙"}) {
		t.Fatalf("the stats name %q", got)
	}
	if s := a.StudyStats(); s.Words != 1 || a.StatsLocked() != 1 {
		t.Fatalf("stats %+v, %d locked", s, a.StatsLocked())
	}
}

// statsWriteBook writes a study record in the library folder dir with the words given.
func statsWriteBook(t *testing.T, dir string, words ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	book, err := recordstudy.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	book.Device = "mac"
	for _, w := range words {
		book.AddWord(saved.Word{Text: w, Meaning: w})
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
}

// statsOtherDirs is the folders of the other libraries' study records the dialog keeps.
func statsOtherDirs(a *app) []string {
	var dirs []string
	for _, book := range a.vocab.stats.others {
		dirs = append(dirs, book.Dir())
	}
	return dirs
}

// statsApplyLate waits until a read let go after the test went on (done is closed once its last study record is read) has handed its result to the engine, and runs it there with whatever else is waiting. The app is quiet otherwise, so the first update that arrives after done is the read's.
func statsApplyLate(t *testing.T, tt *nsgotest.App, a *app, updates <-chan func(), done <-chan struct{}) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	select {
	case <-done:
	case <-timeout:
		t.Fatal("timed out waiting for the held read")
	}
	for len(updates) == 0 {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for the held read's update")
		case <-time.After(time.Millisecond):
		}
	}
	for len(updates) > 0 {
		fn := <-updates
		onEngine(a, func() {
			fn()
			a.Changed()
		})
	}
	tt.Settle()
}

// TestStatsNameNoLibraryWhenNoneIsCounted opens Study › Study Stats while the only library is locked: nothing is counted, so the dialog names no library, rather than a caption with nothing after it, and says that one locked library is left out.
func TestStatsNameNoLibraryWhenNoneIsCounted(t *testing.T) {
	open := filepath.Join(t.TempDir(), "甲.kando")
	statsWriteBook(t, open, "trickle")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	lock, err := shelf.NewLock(lockPassword)
	if err != nil {
		t.Fatal(err)
	}
	a.library.SetLock(open, lock)
	a.edit(func(s *session) {
		s.LibraryDir = open
		s.Libraries = []string{open}
	})
	refresh(tt, a)
	msgs := testlocale.Messages()
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), groupTitle(a.cx, groupStudy), msgs.VocabStatsMenuItem()))
	if !a.vocab.stats.open {
		t.Fatal("Study Stats did not open the dialog")
	}
	if got := a.StatsLibraries(); len(got) != 0 {
		t.Fatalf("the stats name %q", got)
	}
	if tt.Exists("id:stats-libraries") {
		t.Fatalf("the dialog names the libraries counted while none is: %q", tt.Texts())
	}
	wantNoText(t, tt, msgs.VocabStatsLibraries(""))
	wantText(t, tt, msgs.VocabStatsLocked(1))
}

// TestStatsReadTheOthersAgainAsAnotherLibraryOpens opens Study › Study Stats in 甲 while the read of 丙's study record is held, then opens 乙 with the dialog open: the other libraries are read again, now 甲 and 丙, and the stats count 乙, 甲 and 丙. The first read, let go after that, is dropped, so the dialog keeps the second read's records and not 乙's own from the first.
func TestStatsReadTheOthersAgainAsAnotherLibraryOpens(t *testing.T) {
	parent := t.TempDir()
	first, second, third := filepath.Join(parent, "甲.kando"), filepath.Join(parent, "乙.kando"), filepath.Join(parent, "丙.kando")
	statsWriteBook(t, first, "trickle")
	statsWriteBook(t, second, "meander", "delta")
	statsWriteBook(t, third, "estuary", "silt", "levee")
	held, letGo, heldDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var holding atomic.Bool
	prev := vocabLoadBook
	vocabLoadBook = func(dir string) (saved.Book, error) {
		if dir == third && holding.CompareAndSwap(false, true) {
			close(held)
			<-letGo
			defer close(heldDone)
		}
		return prev(dir)
	}
	t.Cleanup(func() { vocabLoadBook = prev })

	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second, third}
	})
	refresh(tt, a)
	onEngine(a, func() { a.OpenStats(vocabTabStats) })
	refresh(tt, a)
	<-held

	apply(tt, a, func() { a.switchLibrary(second) })
	// The sidebar counts the films of the libraries not open once 乙 opens; its update is waited for too, so the next one after the held read is let go is that read's.
	waitUntil(t, tt, a, updates, func() bool {
		_, counted := a.side.libraryCounts[first]
		return len(a.vocab.stats.others) == 2 && counted
	})
	want := []string{first, third}
	if got := statsOtherDirs(a); !slices.Equal(got, want) {
		t.Fatalf("the second read kept %q, want %q", got, want)
	}

	close(letGo)
	statsApplyLate(t, tt, a, updates, heldDone)
	if got := statsOtherDirs(a); !slices.Equal(got, want) {
		t.Fatalf("after the first read came back the dialog keeps %q, want %q", got, want)
	}
	if got := a.StatsLibraries(); !slices.Equal(got, []string{"乙", "甲", "丙"}) {
		t.Fatalf("the stats name %q", got)
	}
	if s := a.StudyStats(); s.Words != 6 {
		t.Fatalf("the stats count %d words, want 6", s.Words)
	}
	msgs := testlocale.Messages()
	sep := msgs.VocabLibrarySeparator()
	wantText(t, tt, msgs.VocabStatsLibraries("乙"+sep+"甲"+sep+"丙"))
}

// TestStatsDropAReadTheDialogClosedOn closes the stats dialog while the other libraries are still being read: the read that comes back after is thrown away, so no other library's study record is kept with the dialog closed.
func TestStatsDropAReadTheDialogClosedOn(t *testing.T) {
	parent := t.TempDir()
	open, other := filepath.Join(parent, "甲.kando"), filepath.Join(parent, "乙.kando")
	statsWriteBook(t, open, "trickle")
	statsWriteBook(t, other, "meander")
	held, letGo, heldDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	prev := vocabLoadBook
	vocabLoadBook = func(dir string) (saved.Book, error) {
		close(held)
		<-letGo
		defer close(heldDone)
		return prev(dir)
	}
	t.Cleanup(func() { vocabLoadBook = prev })

	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = open
		s.Libraries = []string{open, other}
	})
	refresh(tt, a)
	onEngine(a, func() { a.OpenStats(vocabTabStats) })
	refresh(tt, a)
	<-held
	tt.Press("id:stats-done")
	if a.vocab.stats.open {
		t.Fatal("Done left the dialog open")
	}

	close(letGo)
	statsApplyLate(t, tt, a, updates, heldDone)
	if a.vocab.stats.others != nil {
		t.Fatalf("the closed dialog keeps the study records of %q", statsOtherDirs(a))
	}
}

// TestStatsLeaveOutLibrariesWhoseRecordCannotBeRead opens Study › Study Stats with two other libraries whose study record cannot be read, one not JSON and one a later Kando wrote, and a third that reads: the two are neither counted nor named, nothing of them is counted in part, and their files are left as they were.
func TestStatsLeaveOutLibrariesWhoseRecordCannotBeRead(t *testing.T) {
	parent := t.TempDir()
	open, broken, newer, good := filepath.Join(parent, "甲.kando"), filepath.Join(parent, "乙.kando"), filepath.Join(parent, "丙.kando"), filepath.Join(parent, "丁.kando")
	statsWriteBook(t, open, "trickle")
	statsWriteBook(t, good, "meander", "delta")
	records := map[string][]byte{
		broken: []byte(`{"version": 2, "words": [`),
		newer:  []byte(fmt.Sprintf(`{"version": %d, "words": [{"text": "estuary", "meaning": "estuary"}]}`, recordstudy.Version+1)),
	}
	for dir, data := range records {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, recordstudy.FileName), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var read []string
	prev := vocabLoadBook
	vocabLoadBook = func(dir string) (saved.Book, error) {
		mu.Lock()
		read = append(read, dir)
		mu.Unlock()
		return prev(dir)
	}
	t.Cleanup(func() { vocabLoadBook = prev })

	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = open
		s.Libraries = []string{open, broken, newer, good}
	})
	refresh(tt, a)
	onEngine(a, func() { a.OpenStats(vocabTabStats) })
	waitUntil(t, tt, a, updates, func() bool { return len(a.vocab.stats.others) == 1 })

	mu.Lock()
	if !slices.Contains(read, broken) || !slices.Contains(read, newer) {
		t.Fatalf("the stats read %q, not both unreadable records", read)
	}
	mu.Unlock()
	if got := a.StatsLibraries(); !slices.Equal(got, []string{"甲", "丁"}) {
		t.Fatalf("the stats name %q", got)
	}
	if s := a.StudyStats(); s.Words != 3 {
		t.Fatalf("the stats count %d words, want 3", s.Words)
	}
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.VocabStatsLibraries("甲"+msgs.VocabLibrarySeparator()+"丁"))
	for dir, data := range records {
		if after, err := os.ReadFile(filepath.Join(dir, recordstudy.FileName)); err != nil || !bytes.Equal(after, data) {
			t.Fatalf("the study record of %s changed: %v", filepath.Base(dir), err)
		}
	}
}

// TestVocabExportForAnkiCutsTheSounds exports the vocabulary for Anki with ffmpeg at hand (the fake one): the word added on the picture keeps its line's sound in the library as it is added, the export cuts the line's sound into the folder beside the notes off the engine's goroutine, saying it is busy meanwhile, and the word's note plays it.
func TestVocabExportForAnkiCutsTheSounds(t *testing.T) {
	srv, _ := fakeLLM(t, `{"word":"bye","reading":"/baɪ/","meaning":"再见"}`)
	tools := media.Tools{FFmpeg: fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg"), Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates := vocabTalk(t, tools)
	apply(tt, a, func() { a.LookUpWord(1, "Bye") })
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	tt.Press("id:film-lookup-save")
	w := a.saved.book.Words[0]
	if w.Start != 4*time.Second || w.End != 5*time.Second {
		t.Fatalf("the word keeps its line at %v to %v", w.Start, w.End)
	}
	kept := saved.StoredSound(a.saved.book, w.ID)
	waitUntil(t, tt, a, updates, func() bool { return len(a.saved.keeping) == 0 })
	if !saved.HasSound(kept) {
		t.Fatalf("the word's line kept no sound at %s", kept)
	}
	apply(tt, a, func() { a.OpenStats(vocabTabExport) })
	out := t.TempDir()
	tt.PickDirectories = func(nsgotest.DirectoryPick) []ns.Directory { return []ns.Directory{{Path: out}} }
	tt.Press("id:stats-export")
	if !a.vocab.stats.exported.Busy {
		t.Fatalf("the export for Anki is %+v", a.vocab.stats.exported)
	}
	wantText(t, tt, testlocale.Messages().VocabExporting())
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.stats.exported.Busy })
	if e := a.vocab.stats.exported; e.File != filepath.Join(out, "Kando Vocabulary.txt") || e.Err != "" {
		t.Fatalf("exported %+v", e)
	}
	sound := saved.SoundName(saved.WordLine(w))
	notes, _ := os.ReadFile(filepath.Join(out, "Kando Vocabulary.txt"))
	if !strings.Contains(string(notes), "[sound:"+sound+"]") {
		t.Fatalf("the notes read %q", notes)
	}
	if !saved.HasSound(filepath.Join(out, saved.VocabAudio, sound)) {
		t.Fatal("no sound beside the notes")
	}
}

// TestLookupNamesTheFilmsTerm checks which of the open film's glossary terms the card of a word names: a term that is the word, in any case, as clicked or as its dictionary form, before one that has the word among its words; none when no term has it.
func TestLookupNamesTheFilmsTerm(t *testing.T) {
	terms := []core.Term{{Source: "Mekong River", Target: "湄公河"}, {Source: "River", Target: "河流"}, {Source: "meander", Target: "河曲"}}
	for _, c := range []struct {
		words []string
		want  string
	}{
		{[]string{"river"}, "River → 河流"},
		{[]string{"Mekong"}, "Mekong River → 湄公河"},
		{[]string{"meanders", "meander"}, "meander → 河曲"},
		{[]string{"delta"}, ""},
		{[]string{""}, ""},
	} {
		if got := vocabTerm(terms, c.words...); got != c.want {
			t.Errorf("vocabTerm(%q) = %q, want %q", c.words, got, c.want)
		}
	}
}
