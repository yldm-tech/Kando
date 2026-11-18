package study

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestAddWordKeepsOneOfEachWord checks that a word goes into the vocabulary once, whatever its case, with an id, the device it was added on, due at once and its fields stamped; that it is found and removed by its text, leaving a removal behind; and that one added again after is a word of its own.
func TestAddWordKeepsOneOfEachWord(t *testing.T) {
	clock := 0
	b := bookOn("mac", &clock)
	if !b.AddWord(Word{Text: " Trickle ", Reading: "/ˈtrɪk(ə)l/", Meaning: "细流", Language: "简体中文", Context: "Every river begins as a trickle.", Video: "a", Start: time.Second, Film: "Rivers"}) {
		t.Fatal("the word was not added")
	}
	w := b.Words[0]
	if w.ID == "" || w.Device != "mac" || w.Text != "Trickle" || !w.SavedAt.Equal(at(0)) || !w.Due.Equal(at(0)) || w.Box != 0 {
		t.Fatalf("added %+v", w)
	}
	if w.Stamps.Meaning != (Stamp{At: at(0), Device: "mac"}) || w.Stamps.Review != w.Stamps.Meaning {
		t.Fatalf("stamps %+v", w.Stamps)
	}
	clock = 1
	if b.AddWord(Word{Text: "trickle"}) || len(b.Words) != 1 {
		t.Fatalf("the same word in another case was added again: %+v", b.Words)
	}
	if b.WordIndex("TRICKLE") != 0 || b.WordIndex("river") != -1 || b.WordIndex("") != -1 {
		t.Fatal("WordIndex")
	}
	id := w.ID
	clock = 2
	if !b.RemoveWord("trickle") || len(b.Words) != 0 || b.RemoveWord("trickle") {
		t.Fatalf("remove: %+v", b.Words)
	}
	if len(b.RemovedWords) != 1 || b.RemovedWords[0].ID != id || b.RemovedWords[0].At != (Stamp{At: at(2), Device: "mac"}) {
		t.Fatalf("removed %+v", b.RemovedWords)
	}
	clock = 3
	b.AddWord(Word{Text: "trickle"})
	if b.Words[0].ID == id {
		t.Fatal("the word added again took its old id")
	}
}

// TestGradeWordFollowsTheSchedule checks that a word known moves up a box and is due the box's interval later, and one not known goes back to box 0, due at once, stamped when it was reviewed.
func TestGradeWordFollowsTheSchedule(t *testing.T) {
	clock := 0
	b := bookOn("mac", &clock)
	b.AddWord(Word{Text: "trickle"})
	if !b.GradeWord("trickle", true, at(10)) {
		t.Fatal("no word")
	}
	if w := b.Words[0]; w.Box != 1 || !w.Due.Equal(at(10).Add(Intervals[0])) || w.Stamps.Review != (Stamp{At: at(10), Device: "mac"}) {
		t.Fatalf("after good: %+v", w)
	}
	b.GradeWord("trickle", false, at(20))
	if w := b.Words[0]; w.Box != 0 || !w.Due.Equal(at(20)) {
		t.Fatalf("after again: %+v", w)
	}
	if b.GradeWord("river", true, at(30)) {
		t.Fatal("graded a word the book does not have")
	}
}

// TestMergeWordsTakesEachFieldsLatest checks the merge of two devices' vocabularies: a word comes over with its id, the meaning written last and the review done last both stay, a word removed on one device goes on the other and does not come back, a word both added on their own becomes one under the smaller id, and merging again changes nothing.
func TestMergeWordsTakesEachFieldsLatest(t *testing.T) {
	clock := 0
	mac := bookOn("mac", &clock)
	phone := bookOn("phone", &clock)
	mac.AddWord(Word{ID: "b-mac", Text: "trickle", Meaning: "细流"})
	if !phone.MergeWords(mac.Words, mac.RemovedWords) || len(phone.Words) != 1 || phone.Words[0].ID != "b-mac" {
		t.Fatalf("phone took %+v", phone.Words)
	}
	clock = 5
	phone.GradeWord("trickle", true, at(5))
	clock = 6
	mac.SetWordMeaning("trickle", "/ˈtrɪk(ə)l/", "涓涓细流")
	mac.MergeWords(phone.Words, phone.RemovedWords)
	phone.MergeWords(mac.Words, mac.RemovedWords)
	for name, b := range map[string]Book{"mac": mac, "phone": phone} {
		w := b.Words[0]
		if w.Meaning != "涓涓细流" || w.Reading != "/ˈtrɪk(ə)l/" || w.Box != 1 {
			t.Fatalf("%s has %+v", name, w)
		}
	}
	if mac.MergeWords(phone.Words, phone.RemovedWords) || phone.MergeWords(mac.Words, mac.RemovedWords) {
		t.Fatal("merging again changed a book")
	}

	clock = 7
	mac.AddWord(Word{ID: "z-mac", Text: "river"})
	phone.AddWord(Word{ID: "a-phone", Text: "River", Meaning: "河"})
	mac.MergeWords(phone.Words, phone.RemovedWords)
	phone.MergeWords(mac.Words, mac.RemovedWords)
	for name, b := range map[string]Book{"mac": mac, "phone": phone} {
		i := b.WordIndex("river")
		if len(b.Words) != 2 || i < 0 || b.Words[i].ID != "a-phone" || b.Words[i].Meaning != "河" {
			t.Fatalf("%s has %+v", name, b.Words)
		}
		if !slices.ContainsFunc(b.RemovedWords, func(r Removed) bool { return r.ID == "z-mac" }) {
			t.Fatalf("%s removed %+v", name, b.RemovedWords)
		}
	}

	stale := slices.Clone(phone.Words)
	clock = 8
	mac.RemoveWord("trickle")
	if !phone.MergeWords(mac.Words, mac.RemovedWords) || phone.WordIndex("trickle") >= 0 {
		t.Fatalf("the phone kept %+v", phone.Words)
	}
	if mac.MergeWords(stale, nil) || mac.WordIndex("trickle") >= 0 {
		t.Fatalf("a removed word came back: %+v", mac.Words)
	}
}

// TestWordsRoundTrip checks that the vocabulary, its removals and the time watched are kept in the file and read back as they were.
func TestWordsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	clock := 0
	b, _ := Load(dir)
	b.Device = "mac"
	b.Now = func() time.Time { return at(clock) }
	b.AddWord(Word{Text: "trickle", Reading: "/ˈtrɪk(ə)l/", Meaning: "细流", Language: "简体中文", Context: "Every river begins as a trickle.", ContextTranslation: "每条河都始于一股细流。", Video: b.Key(filepath.Join(dir, "Rivers", "video.mp4")), Start: 2300 * time.Millisecond, Film: "Rivers"})
	b.AddWord(Word{Text: "gone"})
	clock = 1
	b.RemoveWord("gone")
	b.GradeWord("trickle", true, at(2))
	b.Watch(at(3), 90*time.Second)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Words) != 1 || len(got.RemovedWords) != 1 || len(got.Watched) != 1 {
		t.Fatalf("read back %+v, %+v, %+v", got.Words, got.RemovedWords, got.Watched)
	}
	w, want := got.Words[0], b.Words[0]
	if w.ID != want.ID || w.Text != "trickle" || w.Reading != want.Reading || w.Meaning != "细流" || w.Language != "简体中文" || w.Context != want.Context || w.ContextTranslation != want.ContextTranslation ||
		w.Video != "Rivers/video.mp4" || w.Start != 2300*time.Millisecond || w.Film != "Rivers" || w.Box != 1 || !w.Due.Equal(want.Due) || !w.SavedAt.Equal(at(0)) ||
		w.Device != "mac" || !w.Stamps.Review.At.Equal(at(2)) || !w.Stamps.Meaning.At.Equal(at(0)) {
		t.Fatalf("word read back as %+v, want %+v", w, want)
	}
	if r := got.RemovedWords[0]; r.ID != b.RemovedWords[0].ID || !r.At.At.Equal(at(1)) || r.At.Device != "mac" {
		t.Fatalf("removal read back as %+v", r)
	}
	if got.Watched[0] != (Watched{Day: Day(at(3)), Device: "mac", Time: 90 * time.Second}) {
		t.Fatalf("watched read back as %+v", got.Watched)
	}
}
