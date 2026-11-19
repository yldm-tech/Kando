package lookup

import (
	"context"
	"errors"
	"testing"
)

// fakeDictionary has the words of entries, by word, and counts the lookups asked of it; with err set, every lookup fails with it.
type fakeDictionary struct {
	id      string
	entries map[string]Entry
	err     error
	asked   int
}

func (d *fakeDictionary) ID() string { return d.id }

func (d *fakeDictionary) LookUp(word, base, reading string) (Entry, bool, error) {
	d.asked++
	if d.err != nil {
		return Entry{}, false, d.err
	}
	e, ok := d.entries[word]
	return e, ok, nil
}

// fakeAI answers every lookup with reply, or fails with err when set, and counts them.
type fakeAI struct {
	reply string
	err   error
	asked int
}

func (f *fakeAI) Chat(ctx context.Context, system, user string) (string, error) {
	f.asked++
	return f.reply, f.err
}

// TestAnswerAsksDictionariesThenTheAIService checks the order a lookup is answered in: the dictionaries in the language it answers in first, in order; then the AI service, which keeps the dictionaries in another language from being asked; those only when no AI service can answer; and ErrNoAnswer when none has the word and no AI service can.
func TestAnswerAsksDictionariesThenTheAIService(t *testing.T) {
	first := &fakeDictionary{id: "first", entries: map[string]Entry{"hello": {Word: "hello", Meaning: "第一"}}}
	second := &fakeDictionary{id: "second", entries: map[string]Entry{"hello": {Word: "hello", Meaning: "第二"}, "bye": {Word: "bye", Meaning: "再见"}}}
	other := &fakeDictionary{id: "other", entries: map[string]Entry{"river": {Word: "river", Meaning: "a stream"}}}
	ai := &fakeAI{reply: `{"word":"river","meaning":"河"}`}
	ask := func(word string, withAI bool) (Entry, string, error) {
		k := Ask{Query: Query{Word: word, Line: word}, Dicts: []Dictionary{first, second}, Others: []Dictionary{other}}
		if withAI {
			k.AI = ai
		}
		return k.Answer(context.Background())
	}
	if e, id, err := ask("hello", true); err != nil || id != "first" || e.Meaning != "第一" || ai.asked != 0 {
		t.Fatalf("hello: %+v from %q, %v; AI asked %d times", e, id, err, ai.asked)
	}
	if e, id, err := ask("bye", true); err != nil || id != "second" || e.Meaning != "再见" {
		t.Fatalf("bye: %+v from %q, %v", e, id, err)
	}
	if e, id, err := ask("river", true); err != nil || id != "" || e.Meaning != "河" || ai.asked != 1 || other.asked != 0 {
		t.Fatalf("river with an AI service: %+v from %q, %v; AI asked %d times, the other dictionary %d", e, id, err, ai.asked, other.asked)
	}
	if e, id, err := ask("river", false); err != nil || id != "other" || e.Meaning != "a stream" {
		t.Fatalf("river without an AI service: %+v from %q, %v", e, id, err)
	}
	if _, _, err := ask("nothing", false); !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("a word no dictionary has, without an AI service: %v", err)
	}
	if (Ask{}).Answers() || !(Ask{Others: []Dictionary{other}}).Answers() || !(Ask{AI: ai}).Answers() {
		t.Fatal("Answers")
	}
}

// TestAnswerFallsBackWhenTheAIServiceFails checks that when the AI service set fails, or gives a reply that is no entry, the dictionaries in another language, which it kept from being asked, answer; that when none of them has the word the AI service's error is the answer; that a lookup given up asks none of them; and that a dictionary whose lookup fails is reported (Failed) and the next one asked.
func TestAnswerFallsBackWhenTheAIServiceFails(t *testing.T) {
	down := errors.New("503 Service Unavailable")
	other := &fakeDictionary{id: "other", entries: map[string]Entry{"river": {Word: "river", Meaning: "a stream"}}}
	for _, ai := range []*fakeAI{{err: down}, {reply: `{"word":"river","meaning":""}`}} {
		k := Ask{Query: Query{Word: "river", Line: "the river"}, Others: []Dictionary{other}, AI: ai}
		if e, id, err := k.Answer(context.Background()); err != nil || id != "other" || e.Meaning != "a stream" || ai.asked != 1 {
			t.Fatalf("the AI service failing (%+v): %+v from %q, %v", ai, e, id, err)
		}
		k.Query.Word = "sea"
		if _, _, err := k.Answer(context.Background()); err == nil || errors.Is(err, ErrNoAnswer) {
			t.Fatalf("a word no dictionary has, the AI service failing: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	asked := other.asked
	k := Ask{Query: Query{Word: "river"}, Others: []Dictionary{other}, AI: &fakeAI{err: context.Canceled}}
	if _, _, err := k.Answer(ctx); !errors.Is(err, context.Canceled) || other.asked != asked {
		t.Fatalf("a lookup given up: %v, the other dictionary asked %d more times", err, other.asked-asked)
	}

	broken := &fakeDictionary{id: "broken", err: errors.New("the dictionary's index is damaged")}
	var failed []string
	k = Ask{Query: Query{Word: "river"}, Dicts: []Dictionary{broken, other}, Failed: func(d Dictionary, err error) { failed = append(failed, d.ID()+": "+err.Error()) }}
	if e, id, err := k.Answer(context.Background()); err != nil || id != "other" || e.Meaning != "a stream" || len(failed) != 1 || failed[0] != "broken: the dictionary's index is damaged" {
		t.Fatalf("a dictionary failing: %+v from %q, %v; reported %q", e, id, err, failed)
	}
}

// TestJapaneseWordsAreAnnotated checks which words a lookup finds the dictionary form and reading of: those with kana or Han characters, not a word in Latin letters in a Japanese line, for which the larger Japanese dictionary is not read.
func TestJapaneseWordsAreAnnotated(t *testing.T) {
	for word, want := range map[string]bool{"行った": true, "カメラ": true, "公園": true, "OK": false, "123": false} {
		if got := japanese(word); got != want {
			t.Errorf("japanese(%q) = %v, want %v", word, got, want)
		}
	}
}

// TestWordLanguage checks the language a word's dictionaries are chosen by: the film's, else Japanese for kana or for Han characters in a line with kana, Chinese for other Han characters, and none for other words.
func TestWordLanguage(t *testing.T) {
	for _, c := range []struct {
		q    Query
		want string
	}{
		{Query{Word: "river", Spoken: "en-GB"}, "en"},
		{Query{Word: "ねこ"}, "ja"},
		{Query{Word: "公園", Line: "公園に行った"}, "ja"},
		{Query{Word: "公园", Line: "去公园"}, "zh"},
		{Query{Word: "river", Line: "the river"}, ""},
	} {
		if got := WordLanguage(c.q); got != c.want {
			t.Errorf("WordLanguage(%+v) = %q, want %q", c.q, got, c.want)
		}
	}
}

// TestFromPlugin checks a lookup source's answer: null and an entry without a meaning are no entry, something else is an error, and a word it does not give is the word looked up.
func TestFromPlugin(t *testing.T) {
	if _, ok, err := FromPlugin("w", nil); ok || err != nil {
		t.Error("null")
	}
	if _, ok, err := FromPlugin("w", "text"); ok || err == nil {
		t.Error("a string")
	}
	if _, ok, _ := FromPlugin("w", map[string]any{"word": "w"}); ok {
		t.Error("no meaning")
	}
	e, ok, err := FromPlugin("run", map[string]any{"meaning": " to go fast ", "reading": "rʌn"})
	if !ok || err != nil || e != (Entry{Word: "run", Reading: "rʌn", Meaning: "to go fast"}) {
		t.Errorf("entry %+v, %t, %v", e, ok, err)
	}
}
