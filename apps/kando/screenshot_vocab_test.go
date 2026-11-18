package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/lookup"
	"kando/apps/kando/saved"
	recordstudy "kando/packages/study"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsVocab renders the prototype's film, How Rivers Shape Landscapes (j1), held on its first line with "trickle" clicked on the picture and its card over it, looked up before so the AI service is not asked (vocab-lookup); and the study stats dialog over the library, a week of watching on this computer and a phone, three words and two saved lines in the study record, at its two tabs (vocab-stats, vocab-export), which counts the fixture's other libraries too, whose study records are empty.
func TestScreenshotsVocab(t *testing.T) {
	f := newScreenshotFixture(t)
	now := time.Date(2026, 11, 18, 21, 0, 0, 0, time.Local)
	f.shoot(t, "vocab-lookup", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		screenshotLoad(tt, a)
		onEngine(a, func() {
			item, _ := a.openFilm()
			_, line := film.SplitCue(a.playerCues()[0].Text)
			a.vocabCache().Put(vocabQuery(item, a.prefs.language, "trickle", line), lookup.Entry{Word: "trickle", Reading: "/ˈtrɪk(ə)l/", Meaning: "n. 细流，涓涓细水；这里指河流在山上刚形成时的一股小水流。"}, now)
			a.LookUpWord(0, "trickle")
		})
		refresh(tt, a)
		if !tt.Exists("id:film-lookup") {
			t.Fatalf("no card: %q", tt.Texts())
		}
	})
	f.shoot(t, "vocab-stats", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotStudyRecord(t, f, a, now)
		refresh(tt, a)
		onEngine(a, func() { a.OpenStats(vocabTabStats) })
		screenshotStatsRead(t, a, tt, updates)
	})
	f.shoot(t, "vocab-export", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotStudyRecord(t, f, a, now)
		refresh(tt, a)
		onEngine(a, func() { a.OpenStats(vocabTabExport) })
		screenshotStatsRead(t, a, tt, updates)
	})
}

// screenshotStatsRead waits for the study stats dialog to have read the study records of the fixture's other libraries, which it counts with the open one's and names.
func screenshotStatsRead(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
	t.Helper()
	waitUntil(t, tt, a, updates, func() bool { return len(a.vocab.stats.others) == len(protoLibraries)-1 })
}

// screenshotStudyRecord fills the library's study record, which the scenes share, as of now: eleven days of watching on this computer, three on a phone, three words, one mastered and one being learned, and two saved lines.
func screenshotStudyRecord(t *testing.T, f *screenshotFixture, a *app, now time.Time) {
	t.Helper()
	onEngine(a, func() {
		if err := os.Remove(filepath.Join(f.lib, saved.FileName)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		a.saved.now = func() time.Time { return now }
		a.saved.book = saved.Book{}
		a.savedSync()
		book := &a.saved.book
		day := func(back int) string { return recordstudy.Day(now.AddDate(0, 0, -back)) }
		for back, m := range []int{38, 25, 42, 0, 31, 18, 12, 20, 15, 30, 25} {
			if m > 0 {
				book.MergeWatched([]recordstudy.Watched{{Day: day(back), Device: a.saved.device, Time: time.Duration(m) * time.Minute}})
			}
		}
		for back, m := range []int{12, 0, 20, 0, 9} {
			if m > 0 {
				book.MergeWatched([]recordstudy.Watched{{Day: day(back), Device: "iphone", Time: time.Duration(m) * time.Minute}})
			}
		}
		for _, w := range []saved.Word{
			{Text: "trickle", Reading: "/ˈtrɪk(ə)l/", Meaning: "n. 细流", Context: protoCues[0].Original, ContextTranslation: protoCues[0].Translation, Film: "How Rivers Shape Landscapes"},
			{Text: "meander", Reading: "/miˈændə(r)/", Meaning: "n. 河曲；v. 蜿蜒", Film: "How Rivers Shape Landscapes"},
			{Text: "erode", Reading: "/ɪˈrəʊd/", Meaning: "v. 侵蚀", Film: "How Rivers Shape Landscapes"},
		} {
			book.AddWord(w)
		}
		book.Words[2].Box, book.Words[2].Due = recordstudy.MasteredBox, now.AddDate(0, 1, 0)
		book.Words[1].Box, book.Words[1].Due = 2, now.AddDate(0, 0, 2)
		book.Toggle(saved.Line{Video: "x/video.mp4", Start: time.Second, Original: protoCues[1].Original, Translation: protoCues[1].Translation, Film: "How Rivers Shape Landscapes"})
		book.Toggle(saved.Line{Video: "x/video.mp4", Start: 9 * time.Second, Original: protoCues[3].Original, Translation: protoCues[3].Translation, Film: "How Rivers Shape Landscapes"})
		book.Lines[0].Box, book.Lines[0].Due = 1, now.AddDate(0, 0, 1)
		a.savedWrite()
	})
}
