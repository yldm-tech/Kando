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
	if b.WordIndex("TRICKLE", "") != 0 || b.WordIndex("river", "") != -1 || b.WordIndex("", "") != -1 {
		t.Fatal("WordIndex")
	}
	id := w.ID
	clock = 2
	if !b.RemoveWord("trickle", "") || len(b.Words) != 0 || b.RemoveWord("trickle", "") {
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
	if !b.GradeWord("trickle", "", true, at(10)) {
		t.Fatal("no word")
	}
	if w := b.Words[0]; w.Box != 1 || !w.Due.Equal(at(10).Add(Intervals[0])) || w.Stamps.Review != (Stamp{At: at(10), Device: "mac"}) {
		t.Fatalf("after good: %+v", w)
	}
	b.GradeWord("trickle", "", false, at(20))
	if w := b.Words[0]; w.Box != 0 || !w.Due.Equal(at(20)) {
		t.Fatalf("after again: %+v", w)
	}
	if b.GradeWord("river", "", true, at(30)) {
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
	phone.GradeWord("trickle", "", true, at(5))
	clock = 6
	mac.SetWordMeaning("trickle", "", "/ˈtrɪk(ə)l/", "涓涓细流")
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
		i := b.WordIndex("river", "")
		if len(b.Words) != 2 || i < 0 || b.Words[i].ID != "a-phone" || b.Words[i].Meaning != "河" {
			t.Fatalf("%s has %+v", name, b.Words)
		}
		if !slices.ContainsFunc(b.RemovedWords, func(r Removed) bool { return r.ID == "z-mac" }) {
			t.Fatalf("%s removed %+v", name, b.RemovedWords)
		}
	}

	stale := slices.Clone(phone.Words)
	clock = 8
	mac.RemoveWord("trickle", "")
	if !phone.MergeWords(mac.Words, mac.RemovedWords) || phone.WordIndex("trickle", "") >= 0 {
		t.Fatalf("the phone kept %+v", phone.Words)
	}
	if mac.MergeWords(stale, nil) || mac.WordIndex("trickle", "") >= 0 {
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
	b.AddWord(Word{Text: "trickle", Spoken: "en", Reading: "/ˈtrɪk(ə)l/", Meaning: "细流", Language: "简体中文", Context: "Every river begins as a trickle.", ContextTranslation: "每条河都始于一股细流。", Video: b.Key(filepath.Join(dir, "Rivers", "video.mp4")), Start: 2300 * time.Millisecond, End: 4100 * time.Millisecond, Film: "Rivers"})
	b.AddWord(Word{Text: "gone"})
	clock = 1
	b.RemoveWord("gone", "")
	b.GradeWord("trickle", "en", true, at(2))
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
	if w.ID != want.ID || w.Text != "trickle" || w.Spoken != "en" || w.Reading != want.Reading || w.Meaning != "细流" || w.Language != "简体中文" || w.Context != want.Context || w.ContextTranslation != want.ContextTranslation ||
		w.Video != "Rivers/video.mp4" || w.Start != 2300*time.Millisecond || w.End != 4100*time.Millisecond || w.Film != "Rivers" || w.Box != 1 || !w.Due.Equal(want.Due) || !w.SavedAt.Equal(at(0)) ||
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

// TestWordsOfTwoLanguagesAreTwoWords checks that one spelling in the vocabulary of two languages spoken is two words: each is found, graded and removed on its own, the other staying, and two devices that each added one keep both when they merge.
func TestWordsOfTwoLanguagesAreTwoWords(t *testing.T) {
	clock := 0
	b := bookOn("mac", &clock)
	if !b.AddWord(Word{Text: "die", Spoken: "de", Meaning: "定冠词"}) || !b.AddWord(Word{Text: "Die", Spoken: "en", Meaning: "死"}) || len(b.Words) != 2 {
		t.Fatalf("the same spelling of two languages was not added twice: %+v", b.Words)
	}
	if b.AddWord(Word{Text: "DIE", Spoken: "EN"}) {
		t.Fatal("the same word of the same language was added again")
	}
	de, en := b.WordIndex("die", "de"), b.WordIndex("die", "en")
	if de < 0 || en < 0 || de == en || b.Words[de].Meaning != "定冠词" || b.Words[en].Meaning != "死" || b.WordIndex("die", "fr") >= 0 {
		t.Fatalf("WordIndex de %d, en %d in %+v", de, en, b.Words)
	}
	if !b.GradeWord("die", "en", true, at(1)) || b.Words[b.WordIndex("die", "de")].Box != 0 {
		t.Fatal("grading the English word graded the German one")
	}
	if !b.RemoveWord("die", "en") || len(b.Words) != 1 || b.Words[0].Spoken != "de" {
		t.Fatalf("removing the English word left %+v", b.Words)
	}

	mac, phone := bookOn("mac", &clock), bookOn("phone", &clock)
	mac.AddWord(Word{ID: "a-mac", Text: "son", Spoken: "fr", Meaning: "他的"})
	phone.AddWord(Word{ID: "b-phone", Text: "son", Spoken: "es", Meaning: "是"})
	mac.MergeWords(phone.Words, phone.RemovedWords)
	phone.MergeWords(mac.Words, mac.RemovedWords)
	for name, b := range map[string]Book{"mac": mac, "phone": phone} {
		if len(b.Words) != 2 || len(b.RemovedWords) != 0 || b.WordIndex("son", "fr") < 0 || b.WordIndex("son", "es") < 0 {
			t.Fatalf("%s has %+v, removed %+v", name, b.Words, b.RemovedWords)
		}
	}
}

// TestDueWordsComeLongestDueFirst checks that the words due are those due by now, the one due longest first, and a word known is no longer due until its box's interval has passed.
func TestDueWordsComeLongestDueFirst(t *testing.T) {
	clock := 0
	b := bookOn("mac", &clock)
	b.AddWord(Word{Text: "first"})
	clock = 5
	b.AddWord(Word{Text: "second"})
	if got := b.DueWords(at(1)); len(got) != 1 || b.Words[got[0]].Text != "first" {
		t.Fatalf("due at 1: %v of %+v", got, b.Words)
	}
	due := b.DueWords(at(10))
	if len(due) != 2 || b.Words[due[0]].Text != "first" || b.Words[due[1]].Text != "second" {
		t.Fatalf("due at 10: %v of %+v", due, b.Words)
	}
	b.GradeWord("first", "", true, at(10))
	if due := b.DueWords(at(11)); len(due) != 1 || b.Words[due[0]].Text != "second" {
		t.Fatalf("after first was known: %v", due)
	}
	if i := b.WordByID(b.Words[0].ID); i != 0 || b.WordByID("") != -1 || b.WordByID("nope") != -1 {
		t.Fatalf("WordByID: %d", i)
	}
}

// TestMergeWordsKeepsTheLinesEnd checks that a copy of a word that does not know where its line ends, as one a phone sent, takes the end the other copy keeps, whichever merges into which.
func TestMergeWordsKeepsTheLinesEnd(t *testing.T) {
	clock := 0
	mac, phone := bookOn("mac", &clock), bookOn("phone", &clock)
	mac.AddWord(Word{ID: "w1", Text: "trickle", Start: time.Second, End: 3 * time.Second})
	bare := mac.Words[0]
	bare.End = 0
	phone.MergeWords([]Word{bare}, nil)
	if !phone.MergeWords(mac.Words, nil) || phone.Words[0].End != 3*time.Second {
		t.Fatalf("the phone has %+v", phone.Words)
	}
	if mac.MergeWords([]Word{bare}, nil) || mac.Words[0].End != 3*time.Second {
		t.Fatalf("the mac has %+v", mac.Words)
	}
}
