package main

import (
	"embed"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The App Store's screenshots: go tool nsgo test -target ios_simulator -device <an iPhone, then an iPad> -run TestScreenshotsStore -env KANDO_SCREENSHOTS=1 saves them as store-*.png. Unlike the other scenes they show a library a learner would have: films with posters, a talk in Japanese translated into English whose video really plays (an original animation, testdata/store/talk.mp4), its words marked, words saved from it, some due for review, and days of watching behind it.

//go:embed testdata/store
var storeAssets embed.FS

// storeAsset is the file name of testdata/store.
func storeAsset(t *testing.T, name string) []byte {
	t.Helper()
	data, err := storeAssets.ReadFile("testdata/store/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// storeCues are the talk's lines: Japanese, translated into English, timed to its animation.
var storeCues = []sync.Cue{
	{StartMS: 1000, EndMS: 2800, Original: "おはようございます。", Translation: "Good morning."},
	{StartMS: 3000, EndMS: 4800, Original: "お元気ですか？", Translation: "How are you doing?"},
	{StartMS: 5000, EndMS: 6800, Original: "元気です。ありがとう！", Translation: "I'm well, thank you!"},
	{StartMS: 7000, EndMS: 9500, Original: "じゃあ、また明日。", Translation: "Well then, see you tomorrow."},
}

// storeMarks are the marks a computer computes for storeCues: readings over the kanji, a level's box under 元気 and 明日, and a word of the vocabulary under おはよう. Starts and ends are byte offsets.
var storeMarks = sync.FilmMarks{Lines: []sync.LineMarks{
	{Line: 0, Hash: sync.LineHash(storeCues[0].Original), Marks: []sync.Mark{{Start: 0, End: 12, Underline: true}}},
	{Line: 1, Hash: sync.LineHash(storeCues[1].Original), Marks: []sync.Mark{{Start: 3, End: 9, Tone: sync.ToneAccent}, {Start: 3, End: 9, Ruby: "げんき"}}},
	{Line: 2, Hash: sync.LineHash(storeCues[2].Original), Marks: []sync.Mark{{Start: 0, End: 6, Tone: sync.ToneAccent}, {Start: 0, End: 6, Ruby: "げんき"}}},
	{Line: 3, Hash: sync.LineHash(storeCues[3].Original), Marks: []sync.Mark{{Start: 18, End: 24, Tone: sync.ToneWarn}, {Start: 18, End: 24, Ruby: "あした"}}},
}}

// storeTalk is the talk as the computer lists it.
var storeTalk = sync.Film{ID: "f1", Title: "Morning Talk", Source: "youtube", DurationMS: 10_000, Spoken: "ja", Translation: "English", Updated: now()}

// storeDesktop is a computer named Studio with two libraries of films with posters, the talk's words and an explanation of its second line, and words saved from it over the past days.
func storeDesktop(t *testing.T) *synctest.Desktop {
	t.Helper()
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese")
	d.AddLibrary("docs", "Documentaries")
	d.AddFilm(synctest.Film{Library: "lib", Video: storeAsset(t, "talk.mp4"), Poster: storeAsset(t, "poster-talk.jpg"), Cues: storeCues, Film: storeTalk})
	other := func(id, library, title, source, poster string, duration time.Duration, ago time.Duration) {
		d.AddFilm(synctest.Film{Library: library, Video: []byte("v"), Poster: storeAsset(t, poster),
			Film: sync.Film{ID: id, Title: title, Source: source, DurationMS: duration.Milliseconds(), Spoken: "ja", Translation: "English", Updated: now().Add(-ago)}})
	}
	other("f2", "lib", "A Walk Through Old Kyoto", "youtube", "poster-kyoto.jpg", 14*time.Minute+32*time.Second, time.Hour)
	other("f3", "lib", "Ramen from Scratch", "bilibili", "poster-ramen.jpg", 9*time.Minute+5*time.Second, 3*time.Hour)
	other("f4", "docs", "Life in the Deep Ocean", "youtube", "poster-ocean.jpg", 48*time.Minute+10*time.Second, 5*time.Hour)
	other("f5", "lib", "Tokyo After Dark", "youtube", "poster-city.jpg", 22*time.Minute+18*time.Second, 26*time.Hour)
	other("f6", "docs", "Vectors, Visually", "bilibili", "poster-algebra.jpg", 17*time.Minute+44*time.Second, 50*time.Hour)
	d.SetMarks("f1", storeMarks)
	d.SetService("OpenAI", "gpt-4.1-mini")
	d.SetModel("gpt-4.1-mini")
	d.SetEntry("元気", sync.WordEntry{Word: "元気", Reading: "げんき", Meaning: "adj. well, healthy; full of energy. お元気ですか asks how someone has been.", Language: "English", Source: sync.WordSource{Kind: sync.SourceAI, Desktop: "Studio"}})
	d.SetExplanationSections(
		sync.ExplainSection{Kind: sync.ExplainMeaning, Text: "A polite \"How are you?\", asked of someone you see again after a while."},
		sync.ExplainSection{Kind: sync.ExplainBreakdown, Text: "お (polite prefix) + 元気 (well, healthy) + です (is) + か (question)"},
		sync.ExplainSection{Kind: sync.ExplainTone, Text: "Friendly and polite. Between close friends, just 元気？ is enough."},
	)
	records := append(storeWords(now().Add(time.Hour)),
		sync.Record{Kind: sync.KindPosition, ID: "f2", Film: "f2", Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(372_000)}}})
	d.SetState("lib", now().Add(-time.Hour), records...)
	d.Offer("123456")
	return d
}

// storeWords are the words saved from the talk over the past hours, the latest first, each due for review at due.
func storeWords(due time.Time) []sync.Record {
	word := func(id, text, reading, meaning, context string, saved time.Duration) sync.Record {
		return sync.Record{Kind: sync.KindWord, ID: id, Film: "f1", Fields: map[string]sync.Field{
			sync.FieldWord: field(text), sync.FieldReading: field(reading), sync.FieldMeaning: field(meaning), sync.FieldSpoken: field("ja"),
			sync.FieldContext: field(context), sync.FieldTitle: field("Morning Talk"), sync.FieldStartMS: field(1000),
			sync.FieldSavedAt: field(now().Add(-saved)), sync.FieldReview: field(sync.Review{Due: due}),
		}}
	}
	return []sync.Record{
		word("w-ohayo", "おはよう", "ohayō", "good morning", "おはようございます。", 2*time.Hour),
		word("w-genki", "元気", "げんき", "well, healthy", "お元気ですか？", 3*time.Hour),
		word("w-ashita", "明日", "あした", "tomorrow", "じゃあ、また明日。", 4*time.Hour),
		word("w-mata", "また", "mata", "again", "じゃあ、また明日。", 5*time.Hour),
		word("w-eki", "駅", "えき", "station", "駅はどこですか。", 6*time.Hour),
		word("w-kawa", "川", "かわ", "river", "川がきれいですね。", 7*time.Hour),
	}
}

// storeKeep puts the talk from d on the phone in dir, downloaded with its poster and left at at, and days of watching behind it.
func storeKeep(t *testing.T, d *synctest.Desktop, dir string, at time.Duration) store.Film {
	t.Helper()
	st := store.Open(dir)
	film := storeTalk
	video := storeAsset(t, "talk.mp4")
	film.VideoSize = int64(len(video))
	f := store.Film{Desktop: store.DesktopID(d.Fingerprint), Library: "Japanese", Video: "video.mp4", Poster: "poster.jpg", Added: now(), PositionMS: at.Milliseconds(), Film: film}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(f.Desktop, f.Film.ID, storeCues); err != nil {
		t.Fatal(err)
	}
	folder := st.FilmDir(f.Desktop, f.Film.ID)
	if err := os.WriteFile(filepath.Join(folder, f.Video), video, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, f.Poster), storeAsset(t, "poster-talk.jpg"), 0o600); err != nil {
		t.Fatal(err)
	}
	watchedFor(t, dir, 18, 25, 32, 20, 41, 28, 26)
	return f
}

// storePhone starts the phone with the talk kept and paired with d, at the Library.
func storePhone(t *testing.T, at time.Duration, opts ...nsgotest.Option) (*phone, *synctest.Desktop, store.Film) {
	t.Helper()
	d := storeDesktop(t)
	dir := t.TempDir()
	f := storeKeep(t, d, dir, at)
	p := startPhone(t, dir, append([]nsgotest.Option{pairedWith(t, d, dir)}, opts...)...)
	p.app.WaitFor(talkRow(f.Desktop), wait)
	return p, d, f
}

func TestScreenshotsStoreLibrary(t *testing.T) {
	shot := screenshots(t)
	p, _, f := storePhone(t, 3500*time.Millisecond)
	p.app.WaitFor("id:films-continue-"+f.Desktop+"-f1", wait)
	p.app.WaitFor(filmRow(f.Desktop, "f6"), wait)
	shot(p, "store-library")
}

func TestScreenshotsStorePlayer(t *testing.T) {
	shot := screenshots(t)
	p, _, f := storePhone(t, 3500*time.Millisecond)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitFor("id:player-card >> id:player-ruby >> text:げんき", time.Minute)
	storeCloseRecap(p)
	shot(p, "store-player")
	// 元気 shows under its reading before the dictionary that cuts Japanese is read in the background, but a tap looks it up only once it is: the line is cut into its words then, です among them.
	p.app.WaitUntil(time.Minute, func() bool { return p.app.Exists(`id:player-card >> text:"です"`) })
	p.app.Press("id:player-card >> text:元気")
	p.app.WaitFor("id:player-word-meaning", wait)
	// The sheet slides up over its transition.
	p.app.Advance(600 * time.Millisecond)
	shot(p, "store-word")
	p.app.Back()
	p.app.WaitForGone("id:player-word-sheet", wait)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-note", wait)
	p.app.Advance(600 * time.Millisecond)
	shot(p, "store-explain")
}

// storeCloseRecap closes the recap of what was watched that a film left partway opens with, leaving the player held where it was left.
func storeCloseRecap(p *phone) {
	p.t.Helper()
	p.app.WaitFor("id:player-recap-close", wait)
	p.app.Press("id:player-recap-close")
	p.app.WaitForGone("id:player-recap", wait)
	// The sheet slides away over its transition.
	p.app.Advance(600 * time.Millisecond)
}

// storeLearnPhone starts the phone with the talk kept and paired with the store's computer, whose words saved from the talk have the three saved first due for review and the others due in days, at Learn.
func storeLearnPhone(t *testing.T) *phone {
	t.Helper()
	d := storeDesktop(t)
	words := storeWords(now().Add(-time.Hour))
	for i := range words[3:] {
		words[3+i].Fields[sync.FieldReview] = field(sync.Review{Box: 2, Due: now().Add(time.Duration(2+i) * 24 * time.Hour)})
	}
	d.SetState("lib", now().Add(-30*time.Minute), words...)
	dir := t.TempDir()
	f := storeKeep(t, d, dir, 3500*time.Millisecond)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDueWords(3) })
	return p
}

// Learn's review card over the week's watching, and the review's card turned over.
func TestScreenshotsStoreLearn(t *testing.T) {
	shot := screenshots(t)
	p := storeLearnPhone(t)
	shot(p, "store-learn")
	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-card", wait)
	p.app.Press("id:review-show")
	p.app.WaitFor("id:review-answer", wait)
	// The card turns over in half a second.
	p.app.Advance(600 * time.Millisecond)
	shot(p, "store-review")
}

// The words saved from the talk, with those due for review marked.
func TestScreenshotsStoreWords(t *testing.T) {
	shot := screenshots(t)
	p := storeLearnPhone(t)
	p.app.WaitFor("id:learn-word-w-kawa", wait)
	p.learnScrollTo("id:learn-vocabulary-section")
	shot(p, "store-words")
}
