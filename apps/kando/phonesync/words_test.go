package phonesync

import (
	"testing"
	"time"

	"kando/packages/study"
	psync "kando/packages/sync"
)

func TestWordRecordsReadBackIntoTheStudyRecord(t *testing.T) {
	saved := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	book := study.Book{Device: "desk", Now: func() time.Time { return saved }}
	book.AddWord(study.Word{Text: "erosion", Spoken: "en", Reading: "/ɪˈroʊʒən/", Meaning: "侵蚀", Language: "简体中文", Context: "Erosion shapes the valley.", ContextTranslation: "侵蚀塑造了山谷。", Video: "Talk/talk.mp4", Start: 1200 * time.Millisecond, End: 3400 * time.Millisecond, Film: "Talk"})
	book.AddWord(study.Word{Text: "gone", Spoken: "en"})
	book.RemoveWord("gone", "en")
	films := map[string]string{"Talk/talk.mp4": "f1"}
	records := WordRecords(book, func(video string) string { return films[video] })
	if len(records) != 2 {
		t.Fatalf("records %+v, want a word and a tombstone", records)
	}
	word, gone := records[0], records[1]
	if word.Kind != psync.KindWord || word.Film != "f1" || string(word.Fields[psync.FieldStartMS].Value) != "1200" || string(word.Fields[psync.FieldEndMS].Value) != "3400" || word.Fields[psync.FieldMeaning].Device != "desk" {
		t.Errorf("word %+v", word)
	}
	if gone.Removed == nil {
		t.Errorf("tombstone %+v", gone)
	}
	if why := StateAccepts(records, func(string) bool { return false }, func(string, string) bool { return false }); why != "" {
		t.Errorf("the desktop's own words are refused: %s", why)
	}

	// The same records taken into a book of another device make the same word, and its removal.
	other := study.Book{Device: "phone"}
	words, removed := WordsOf(other, records, func(film string) (string, bool) { return "Talk/talk.mp4", film == "f1" })
	other.MergeWords(words, removed)
	if len(other.Words) != 1 || len(other.RemovedWords) != 1 {
		t.Fatalf("book %+v", other)
	}
	got, want := other.Words[0], book.Words[0]
	if got.ID != want.ID || got.Text != want.Text || got.Spoken != want.Spoken || got.Reading != want.Reading || got.Meaning != want.Meaning || got.Language != want.Language ||
		got.Context != want.Context || got.ContextTranslation != want.ContextTranslation || got.Video != want.Video || got.Start != want.Start || got.End != want.End || got.Film != want.Film ||
		!got.SavedAt.Equal(want.SavedAt) || got.Device != "desk" || !got.Due.Equal(want.Due) {
		t.Errorf("word read back %+v, want %+v", got, want)
	}

	// A phone's review of the word moves it up a box on the desktop.
	due := saved.Add(24 * time.Hour)
	review := psync.Record{Kind: psync.KindWord, ID: want.ID, Fields: map[string]psync.Field{psync.FieldReview: {Value: psync.Value(psync.Review{Box: 1, Due: due}), Stamp: psync.Stamp{At: saved.Add(time.Hour), Device: "phone"}}}}
	words, removed = WordsOf(book, []psync.Record{review}, func(string) (string, bool) { return "", false })
	if !book.MergeWords(words, removed) || book.Words[0].Box != 1 || !book.Words[0].Due.Equal(due) || book.Words[0].Meaning != "侵蚀" {
		t.Errorf("the review was taken in as %+v", book.Words[0])
	}

	// A word whose line's end is not known, as one added before ends were kept, goes without one, and a copy read back without it takes the end of the copy that has it.
	book.AddWord(study.Word{Text: "valley", Spoken: "en", Video: "Talk/talk.mp4", Start: 1200 * time.Millisecond})
	old := WordRecords(book, func(video string) string { return films[video] })[0]
	if _, ok := old.Fields[psync.FieldEndMS]; ok || old.Fields[psync.FieldWord].Value == nil {
		t.Errorf("a word without its line's end is sent with one: %+v", old)
	}
	phone := study.Book{Device: "phone"}
	words, removed = WordsOf(phone, []psync.Record{old}, func(string) (string, bool) { return "Talk/talk.mp4", true })
	phone.MergeWords(words, removed)
	withEnd := phone.Words[0]
	withEnd.End = 2900 * time.Millisecond
	if !book.MergeWords([]study.Word{withEnd}, nil) || book.Words[0].End != 2900*time.Millisecond {
		t.Errorf("the desktop's copy did not take the phone's end: %+v", book.Words[0])
	}

	// A new word without its word, or with a meaning that is not text, is refused.
	bad := []psync.Record{{Kind: psync.KindWord, ID: "x", Fields: map[string]psync.Field{psync.FieldMeaning: {Value: psync.Value("x")}}}}
	if StateAccepts(bad, func(string) bool { return true }, func(string, string) bool { return false }) == "" {
		t.Error("a word without its word was taken")
	}
	if FieldValid(psync.KindWord, psync.FieldMeaning, psync.Value(3)) {
		t.Error("a meaning that is a number is valid")
	}
	if FieldValid(psync.KindWord, psync.FieldEndMS, psync.Value("3s")) || !FieldValid(psync.KindWord, psync.FieldEndMS, psync.Value(3400)) {
		t.Error("a word's end is not checked to be a number")
	}
}
