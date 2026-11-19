package learning

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	phonestudy "kando/apps/mobile/study"
	"kando/packages/study"
	"kando/packages/sync"
)

var t0 = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

func field(v any, at time.Time, device string) sync.Field {
	return sync.Field{Value: sync.Value(v), Stamp: sync.Stamp{At: at, Device: device}}
}

// word is a word record of the desktop's, due at due in box box.
func word(id, text, meaning string, box int, due time.Time) sync.Record {
	return sync.Record{Kind: sync.KindWord, ID: id, Film: "f1", Fields: map[string]sync.Field{
		sync.FieldWord: field(text, t0, "desk"), sync.FieldMeaning: field(meaning, t0, "desk"), sync.FieldContext: field("A "+text+" here.", t0, "desk"),
		sync.FieldTitle: field("Talk", t0, "desk"), sync.FieldSavedAt: field(t0, t0, "desk"), sync.FieldStartMS: field(1500, t0, "desk"),
		sync.FieldReview: field(sync.Review{Box: box, Due: due}, t0, "desk"),
	}}
}

func line(id, original, translation string, due time.Time) sync.Record {
	return sync.Record{Kind: sync.KindLine, ID: id, Film: "f1", Fields: map[string]sync.Field{
		sync.FieldOriginal: field(original, t0, "desk"), sync.FieldTranslation: field(translation, t0, "desk"), sync.FieldNote: field("", t0, "desk"),
		sync.FieldTitle: field("Talk", t0, "desk"), sync.FieldSavedAt: field(t0.Add(time.Minute), t0, "desk"), sync.FieldStartMS: field(1000, t0, "desk"), sync.FieldEndMS: field(2000, t0, "desk"),
		sync.FieldReview: field(sync.Review{Due: due}, t0, "desk"),
	}}
}

// items are the words and saved lines of l, a library of computer d.
func items(l phonestudy.Library) []Item { return Items("d", "lib", l) }

// grade is l with it graded at now, made from the schedule the computer has, as the session makes it.
func grade(l phonestudy.Library, it Item, good bool, now time.Time) phonestudy.Library {
	return phonestudy.Change(l, phonestudy.Based(l, Grade(it, good, now, "phone")))
}

func TestItemsAreTheWordsAndLinesNotRemoved(t *testing.T) {
	gone := sync.Stamp{At: t0, Device: "desk"}
	l := phonestudy.Apply(phonestudy.Library{}, sync.StateChanges{Epoch: "e", Rev: 3, Reset: true, Records: []sync.Record{
		word("w1", "river", "河", 0, t0), line("l1", "Hello.", "你好。", t0.Add(48*time.Hour)),
		{Kind: sync.KindWord, ID: "w2", Removed: &gone},
		{Kind: sync.KindPosition, ID: "f1", Film: "f1", Fields: map[string]sync.Field{sync.FieldPositionMS: field(1, t0, "desk")}},
	}})
	got := items(l)
	if len(got) != 2 || got[0].ID != "l1" || got[1].ID != "w1" {
		t.Fatalf("items %+v, want the line saved last first, then the word", got)
	}
	w := got[1]
	if w.Kind != Word || w.Text != "river" || w.Meaning != "河" || w.Context != "A river here." || w.Start != 1500*time.Millisecond || w.Title != "Talk" || !w.SavedAt.Equal(t0) || w.Desktop != "d" || w.Library != "lib" {
		t.Errorf("word %+v", w)
	}
	if l := got[0]; l.Kind != Line || l.Text != "Hello." || l.Meaning != "你好。" || l.End != 2*time.Second {
		t.Errorf("line %+v", l)
	}
	due := Due(got, t0.Add(time.Hour))
	if len(due) != 1 || due[0].ID != "w1" {
		t.Errorf("due %+v, want the word alone", due)
	}
}

// A review on the phone shows at once, waits to be put, keeps the stamp of the schedule it changed from, and leaves Pending once the computer took it.
func TestAReviewWaitsUntilTheComputerTakesIt(t *testing.T) {
	l := phonestudy.Apply(phonestudy.Library{}, sync.StateChanges{Epoch: "e", Rev: 1, Reset: true, Records: []sync.Record{word("w1", "river", "河", 0, t0)}})
	at := t0.Add(time.Hour)
	l = grade(l, items(l)[0], true, at)
	got := items(l)[0]
	if got.Box != 1 || !got.Due.Equal(at.Add(study.Intervals[0])) {
		t.Fatalf("after a good review %+v", got)
	}
	desk := sync.Stamp{At: t0, Device: "desk"}
	if len(l.Pending) != 1 || l.Pending[0].Fields[sync.FieldReview].Base == nil || !l.Pending[0].Fields[sync.FieldReview].Base.Equal(desk) {
		t.Fatalf("pending %+v", l.Pending)
	}
	// Known again before the computer took the first: the base stays the computer's.
	l = grade(l, items(l)[0], true, at.Add(time.Minute))
	if len(l.Pending) != 1 || items(l)[0].Box != 2 || !l.Pending[0].Fields[sync.FieldReview].Base.Equal(desk) {
		t.Fatalf("pending %+v, items %+v", l.Pending, items(l))
	}
	// The computer's changes meanwhile do not hide the phone's own.
	l = phonestudy.Apply(l, sync.StateChanges{Epoch: "e", Rev: 2, Records: []sync.Record{word("w1", "river", "河流", 0, t0)}})
	if got := items(l)[0]; got.Box != 2 || got.Meaning != "河流" {
		t.Fatalf("after the computer's change %+v", got)
	}
	sent := slices.Clone(l.Pending)
	merged := word("w1", "river", "河流", 2, at.Add(time.Minute).Add(study.Intervals[1]))
	l = phonestudy.Took(l, sent, sync.StatePutResult{Epoch: "e", Rev: 3, Records: []sync.Record{merged}}, false)
	if len(l.Pending) != 0 || items(l)[0].Box != 2 {
		t.Fatalf("settled: pending %+v, items %+v", l.Pending, items(l))
	}
	// Not known puts it back in box 0, due at once.
	if r := Grade(items(l)[0], false, at, "phone"); !strings.Contains(string(r.Fields[sync.FieldReview].Value), `"box":0`) {
		t.Errorf("not known: %s", r.Fields[sync.FieldReview].Value)
	}
}

// A change made while a put is on its way stays pending once the put is answered.
func TestAChangeMadeDuringAPutStaysPending(t *testing.T) {
	l := phonestudy.Apply(phonestudy.Library{}, sync.StateChanges{Epoch: "e", Rev: 1, Reset: true, Records: []sync.Record{word("w1", "river", "河", 0, t0)}})
	l = grade(l, items(l)[0], true, t0.Add(time.Hour))
	sent := slices.Clone(l.Pending)
	l = phonestudy.Sending(l, sent)
	l = phonestudy.Change(l, Removal(items(l)[0], t0.Add(2*time.Hour), "phone"))
	l = phonestudy.Took(l, sent, sync.StatePutResult{Epoch: "e", Rev: 2, Records: []sync.Record{word("w1", "river", "河", 1, t0.Add(25*time.Hour))}}, false)
	if len(l.Pending) != 1 || l.Pending[0].Removed == nil {
		t.Fatalf("pending %+v, want the removal", l.Pending)
	}
	if len(items(l)) != 0 {
		t.Fatalf("a word removed on the phone still shows: %+v", items(l))
	}
}

func TestTheTimeWatchedIsKept(t *testing.T) {
	d := Open(t.TempDir())
	w := Watched{}
	w.Add(t0, 90*time.Second)
	if err := d.SaveWatched(w); err != nil {
		t.Fatal(err)
	}
	back, err := d.Watched()
	if err != nil || back.Day(t0) != 90*time.Second {
		t.Fatalf("watched %+v, %v", back, err)
	}
}

func TestStreakAndWeek(t *testing.T) {
	w := Watched{}
	w.Add(t0.AddDate(0, 0, -2), 5*time.Minute)
	w.Add(t0.AddDate(0, 0, -1), 30*time.Second)
	w.Add(t0.AddDate(0, 0, -1), time.Minute)
	if got := w.Streak(t0); got != 2 {
		t.Errorf("streak before today's study %d, want the run up to yesterday, 2", got)
	}
	w.Add(t0, 2*time.Minute)
	if got := w.Streak(t0); got != 3 {
		t.Errorf("streak %d, want 3", got)
	}
	week := w.Week(t0)
	if week[6] != 2*time.Minute || week[5] != 90*time.Second || week[4] != 5*time.Minute || week[0] != 0 {
		t.Errorf("week %v", week)
	}
	if days := WeekDays(t0); study.Day(days[6]) != study.Day(t0) || study.Day(days[0]) != study.Day(t0.AddDate(0, 0, -6)) {
		t.Errorf("days %v", days)
	}
}

func TestNextReminder(t *testing.T) {
	loc := time.FixedZone("JST", 9*3600)
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, loc)
	at := func(d time.Time) []Item { return []Item{{Due: d}} }
	cases := []struct {
		name  string
		now   time.Time
		items []Item
		want  time.Time
	}{
		{"due now, before nine", now, at(now.Add(-time.Hour)), time.Date(2026, 11, 18, 21, 0, 0, 0, loc)},
		{"due now, after nine", now.Add(12 * time.Hour), at(now), time.Date(2026, 11, 19, 21, 0, 0, 0, loc)},
		{"due later today, before nine", now, at(now.Add(5 * time.Hour)), time.Date(2026, 11, 18, 21, 0, 0, 0, loc)},
		{"due tonight after nine", now, at(time.Date(2026, 11, 18, 22, 0, 0, 0, loc)), time.Date(2026, 11, 19, 21, 0, 0, 0, loc)},
		{"due in three days", now, at(now.AddDate(0, 0, 3)), time.Date(2026, 11, 21, 21, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		got, ok := NextReminder(c.now, c.items)
		if !ok || !got.Equal(c.want) {
			t.Errorf("%s: %v, %v; want %v", c.name, got, ok, c.want)
		}
	}
	if _, ok := NextReminder(now, nil); ok {
		t.Error("a reminder with nothing to review")
	}
}

// The export writes the words in the format and with what the options ask for, the saved lines after them only when asked for, as the computer's export writes a library's.
func TestExportWritesTheWordsAsTheComputerDoes(t *testing.T) {
	items := []Item{
		{Kind: Word, Text: "river", Reading: "/ˈrɪvər/", Meaning: "河", Context: "A river, here.", ContextTranslation: "一条河。", Title: "Talk"},
		{Kind: Line, Text: "Hello.\nAgain.", Meaning: "你好。", Note: "greeting", Title: "Talk"},
	}
	path, err := Export(filepath.Join(t.TempDir(), "export"), items, study.VocabOptions{Format: study.VocabCSV, Context: true, Lines: true})
	if err != nil || filepath.Base(path) != study.VocabFile(study.VocabCSV) {
		t.Fatalf("exported %q, %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "\ufeff") {
		t.Fatalf("read %q, %v; want a byte-order mark", data, err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || strings.Join(rows[0], "|") != "Word|Reading|Meaning|Sentence|Film" || strings.Join(rows[1], "|") != "river|/ˈrɪvər/|河|A river, here.|Talk" || rows[2][0] != "Hello.\nAgain." || rows[2][2] != "你好。\ngreeting" {
		t.Errorf("rows %q", rows)
	}

	path, err = Export(t.TempDir(), items, study.VocabOptions{Format: study.VocabAnki})
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if want := "#separator:tab\n#html:false\n#notetype:Basic\n\"river\"\t\"/ˈrɪvər/\n河\nTalk\"\n"; err != nil || string(data) != want {
		t.Errorf("Anki's notes without the lines read %q, %v; want %q", data, err, want)
	}
}

// A dictation marks each word of the line the learner typed, whatever its case, punctuation and order, each typed word standing for one of the line's; a line in Japanese is checked a character at a time.
func TestDictateMarksTheWordsHeard(t *testing.T) {
	words, hit, total := Dictate("Fine, thanks. Fine!", "fine THANKS")
	if hit != 2 || total != 3 || !words[0].Typed || !words[1].Typed || words[2].Typed || words[0].Text != "Fine," {
		t.Errorf("Dictate = %+v, %d/%d", words, hit, total)
	}
	words, hit, total = Dictate("It’s a well-known fact — 42.", "its a well-known fact 42")
	if hit != 4 || total != 5 || words[0].Typed || words[4].Counted {
		t.Errorf("Dictate = %+v, %d/%d; want the dash left out and it’s not its", words, hit, total)
	}
	words, hit, total = Dictate("おはようございます。", "おはよう")
	if hit != 4 || total != 9 || len(words) != 10 || words[9].Counted {
		t.Errorf("Dictate = %+v, %d/%d", words, hit, total)
	}
	if _, hit, total := Dictate("Hello.", ""); hit != 0 || total != 1 {
		t.Errorf("nothing typed: %d/%d", hit, total)
	}
}

// A line's clip is its own span, a word's the line it was looked up in, which ends where the word's record says or else where the film's subtitles end it, both moved by the film's delay and padded; a word with no line, or a film not on the phone, has none.
func TestClipOf(t *testing.T) {
	cues := []sync.Cue{{StartMS: 1000, EndMS: 2000, Original: "Good morning."}, {StartMS: 3000, EndMS: 4500, Original: "How are you?"}}
	line := Item{Kind: Line, Text: "Fine.", Meaning: "很好。", Start: 5 * time.Second, End: 6500 * time.Millisecond}
	c, ok := ClipOf(line, "/v.mp4", time.Second, cues)
	if !ok || c.From != 5750*time.Millisecond || c.To != 7750*time.Millisecond || c.Line != "Fine." || c.Translation != "很好。" || c.Video != "/v.mp4" {
		t.Errorf("the line's clip %+v, %v", c, ok)
	}
	word := Item{Kind: Word, Text: "how", Meaning: "怎样", Context: "How are you?", ContextTranslation: "你好吗？", Start: 3 * time.Second}
	c, ok = ClipOf(word, "/v.mp4", 0, cues)
	if !ok || c.From != 2750*time.Millisecond || c.To != 4750*time.Millisecond || c.Line != "How are you?" || c.Translation != "你好吗？" {
		t.Errorf("the word's clip %+v, %v", c, ok)
	}
	// A word added with its line's end, as a phone keeps it, plays to that end whatever the subtitles say now.
	word.End = 4 * time.Second
	if c, ok = ClipOf(word, "/v.mp4", 0, cues); !ok || c.To != 4250*time.Millisecond {
		t.Errorf("the word's clip with its line's end %+v, %v", c, ok)
	}
	word.Start, word.End = 9*time.Second, 0
	if c, ok = ClipOf(word, "/v.mp4", 0, cues); !ok || c.To != 13250*time.Millisecond {
		t.Errorf("a line not among the subtitles plays %+v, %v; want four seconds", c, ok)
	}
	if _, ok := ClipOf(Item{Kind: Word, Text: "how"}, "/v.mp4", 0, cues); ok {
		t.Error("a word with no line has a clip")
	}
	if _, ok := ClipOf(line, "", 0, cues); ok {
		t.Error("a line with no video has a clip")
	}
}

// The time watched of a library's other devices adds to the phone's own, the phone's own records there left out; the phone's change adds to what the library has of its day.
func TestWatchedAddsTheOtherDevices(t *testing.T) {
	day := study.Day(t0)
	rec := func(device string, ms int) sync.Record {
		return sync.Record{Kind: sync.KindWatched, ID: sync.WatchedID(device, day), Fields: map[string]sync.Field{sync.FieldWatchedMS: field(ms, t0, device)}}
	}
	l := phonestudy.Library{Records: []sync.Record{rec("mac", 600_000), rec("phone", 120_000), rec("pad", 60_000)}}
	w := Watched{}
	w.Add(t0, 3*time.Minute)
	w.AddOthers(l, func(device string) bool { return device == "phone" })
	if got := w.Day(t0); got != 14*time.Minute {
		t.Errorf("the day's time is %v, want the phone's 3 minutes with the mac's 10 and the pad's 1", got)
	}
	change := WatchedChange(l, "phone", t0, 30*time.Second)
	var ms float64
	if err := json.Unmarshal(change.Fields[sync.FieldWatchedMS].Value, &ms); err != nil || ms != 150_000 || change.ID != sync.WatchedID("phone", day) || change.Fields[sync.FieldWatchedMS].Device != "phone" {
		t.Errorf("the change %+v reads %v, %v", change, ms, err)
	}
}
