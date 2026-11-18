package learning

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestItemsAreTheWordsAndLinesNotRemoved(t *testing.T) {
	m := Mirror{Desktop: "d", Library: "lib"}
	gone := sync.Stamp{At: t0, Device: "desk"}
	m.Take(sync.StateChanges{Epoch: "e", Rev: 3, Reset: true, Records: []sync.Record{
		word("w1", "river", "河", 0, t0), line("l1", "Hello.", "你好。", t0.Add(48*time.Hour)),
		{Kind: sync.KindWord, ID: "w2", Removed: &gone},
		{Kind: sync.KindPosition, ID: "f1", Film: "f1", Fields: map[string]sync.Field{sync.FieldPositionMS: field(1, t0, "desk")}},
	}})
	items := m.Items()
	if len(items) != 2 || items[0].ID != "l1" || items[1].ID != "w1" {
		t.Fatalf("items %+v, want the line saved last first, then the word", items)
	}
	w := items[1]
	if w.Kind != Word || w.Text != "river" || w.Meaning != "河" || w.Context != "A river here." || w.Start != 1500*time.Millisecond || w.Title != "Talk" || !w.SavedAt.Equal(t0) {
		t.Errorf("word %+v", w)
	}
	if l := items[0]; l.Kind != Line || l.Text != "Hello." || l.Meaning != "你好。" || l.End != 2*time.Second {
		t.Errorf("line %+v", l)
	}
	due := Due(items, t0.Add(time.Hour))
	if len(due) != 1 || due[0].ID != "w1" {
		t.Errorf("due %+v, want the word alone", due)
	}
}

// A review on the phone shows at once, waits to be put, keeps the stamp of the schedule it changed from, and leaves Pending once the computer took it.
func TestAReviewWaitsUntilTheComputerTakesIt(t *testing.T) {
	m := Mirror{Desktop: "d", Library: "lib"}
	m.Take(sync.StateChanges{Epoch: "e", Rev: 1, Reset: true, Records: []sync.Record{word("w1", "river", "河", 0, t0)}})
	it := m.Items()[0]
	at := t0.Add(time.Hour)
	m.Change(Grade(it, true, at, "phone"))
	got := m.Items()[0]
	if got.Box != 1 || !got.Due.Equal(at.Add(study.Intervals[0])) {
		t.Fatalf("after a good review %+v", got)
	}
	if len(m.Pending) != 1 || m.Pending[0].Fields[sync.FieldReview].Base == nil || !m.Pending[0].Fields[sync.FieldReview].Base.Equal(sync.Stamp{At: t0, Device: "desk"}) {
		t.Fatalf("pending %+v", m.Pending)
	}
	// Known again before the computer took the first: the base stays the computer's.
	m.Change(Grade(m.Items()[0], true, at.Add(time.Minute), "phone"))
	if len(m.Pending) != 1 || m.Items()[0].Box != 2 || !m.Pending[0].Fields[sync.FieldReview].Base.Equal(sync.Stamp{At: t0, Device: "desk"}) {
		t.Fatalf("pending %+v, items %+v", m.Pending, m.Items())
	}
	// The computer's changes meanwhile do not hide the phone's own.
	m.Take(sync.StateChanges{Epoch: "e", Rev: 2, Records: []sync.Record{word("w1", "river", "河流", 0, t0)}})
	if got := m.Items()[0]; got.Box != 2 || got.Meaning != "河流" {
		t.Fatalf("after the computer's change %+v", got)
	}
	sent := append([]sync.Record(nil), m.Pending...)
	merged := word("w1", "river", "河流", 2, at.Add(time.Minute).Add(study.Intervals[1]))
	m.Settle(sent, sync.StatePutResult{Epoch: "e", Rev: 3, Records: []sync.Record{merged}})
	if len(m.Pending) != 0 || m.Items()[0].Box != 2 {
		t.Fatalf("settled: pending %+v, items %+v", m.Pending, m.Items())
	}
	// Not known puts it back in box 0, due at once.
	if r := Grade(m.Items()[0], false, at, "phone"); !strings.Contains(string(r.Fields[sync.FieldReview].Value), `"box":0`) {
		t.Errorf("not known: %s", r.Fields[sync.FieldReview].Value)
	}
}

// A change made while a put is on its way stays pending once the put is answered.
func TestAChangeMadeDuringAPutStaysPending(t *testing.T) {
	m := Mirror{Desktop: "d", Library: "lib"}
	m.Take(sync.StateChanges{Epoch: "e", Rev: 1, Reset: true, Records: []sync.Record{word("w1", "river", "河", 0, t0)}})
	m.Change(Grade(m.Items()[0], true, t0.Add(time.Hour), "phone"))
	sent := append([]sync.Record(nil), m.Pending...)
	m.Change(Removal(m.Items()[0], t0.Add(2*time.Hour), "phone"))
	m.Settle(sent, sync.StatePutResult{Epoch: "e", Rev: 2, Records: []sync.Record{word("w1", "river", "河", 1, t0.Add(25*time.Hour))}})
	if len(m.Pending) != 1 || m.Pending[0].Removed == nil {
		t.Fatalf("pending %+v, want the removal", m.Pending)
	}
	if len(m.Items()) != 0 {
		t.Fatalf("a word removed on the phone still shows: %+v", m.Items())
	}
}

func TestTheMirrorsAndTheTimeWatchedAreKept(t *testing.T) {
	d := Open(t.TempDir())
	m := Mirror{Desktop: "d", Library: "lib", Name: "Class"}
	m.Take(sync.StateChanges{Epoch: "e", Rev: 1, Reset: true, Records: []sync.Record{word("w1", "river", "河", 0, t0)}})
	m.Change(Grade(m.Items()[0], true, t0, "phone"))
	if err := d.SaveDesktop("d", []Mirror{m}); err != nil {
		t.Fatal(err)
	}
	got, err := d.Mirrors()
	if err != nil || len(got) != 1 || got[0].Name != "Class" || len(got[0].Pending) != 1 || got[0].Items()[0].Box != 1 {
		t.Fatalf("mirrors %+v, %v", got, err)
	}
	w := Watched{}
	w.Add(t0, 90*time.Second)
	if err := d.SaveWatched(w); err != nil {
		t.Fatal(err)
	}
	back, err := d.Watched()
	if err != nil || back.Day(t0) != 90*time.Second {
		t.Fatalf("watched %+v, %v", back, err)
	}
	if err := d.DeleteDesktop("d"); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Mirrors(); len(got) != 0 {
		t.Fatalf("mirrors of a computer forgotten %+v", got)
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

func TestExportWritesATableOfEachKind(t *testing.T) {
	dir := t.TempDir()
	items := []Item{
		{Kind: Word, Text: "river", Reading: "/ˈrɪvər/", Meaning: "河", Context: "A river, here.", ContextTranslation: "一条河。", Title: "Talk"},
		{Kind: Line, Text: "Hello.\nAgain.", Meaning: "你好。", Note: "greeting", Title: "Talk"},
	}
	cols := Columns{Word: "Word", Reading: "Reading", Meaning: "Meaning", Line: "Line", Translation: "Translation", Film: "Film", Note: "Note"}
	files, err := Export(dir, items, cols)
	if err != nil || len(files) != 2 {
		t.Fatalf("files %v, %v", files, err)
	}
	read := func(name string) [][]string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(data), "\ufeff") {
			t.Errorf("%s has no byte-order mark", name)
		}
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	words := read(WordsFile)
	if len(words) != 2 || words[0][0] != "Word" || words[1][0] != "river" || words[1][3] != "A river, here." || words[1][5] != "Talk" {
		t.Errorf("words %q", words)
	}
	lines := read(LinesFile)
	if len(lines) != 2 || lines[1][0] != "Hello.\nAgain." || lines[1][2] != "greeting" {
		t.Errorf("lines %q", lines)
	}
	files, err = Export(t.TempDir(), items[:1], cols)
	if err != nil || len(files) != 1 || filepath.Base(files[0]) != WordsFile {
		t.Errorf("a vocabulary alone wrote %v, %v", files, err)
	}
}
