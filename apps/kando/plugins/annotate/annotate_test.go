package annotate

import (
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/wordlist"
)

// list is a word list of tiers n5 (success) and n3 (accent), read from file.
func list(t *testing.T, file string) *wordlist.List {
	t.Helper()
	l, err := wordlist.Parse(strings.NewReader(file), []plugins.Tier{{ID: "n5", Tone: plugins.ToneSuccess}, {ID: "n3", Tone: plugins.ToneAccent}})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// at is the mark over the run of line that is word, with the fields of want, at the first place word is in line.
func at(line, word string, want Mark) Mark {
	want.Start = strings.Index(line, word)
	want.End = want.Start + len(word)
	return want
}

// TestFurigana checks where a Japanese word's reading goes: over its Han characters, the kana it begins and ends with left out, the long vowel mark and halfwidth katakana among them, and nowhere for a word read as written.
func TestFurigana(t *testing.T) {
	for _, c := range []struct{ word, reading, over, ruby string }{
		{"食べた", "たべた", "食", "た"},
		{"お茶", "おちゃ", "茶", "ちゃ"},
		{"東京", "とうきょう", "東京", "とうきょう"},
		{"取り扱い", "とりあつかい", "取り扱", "とりあつか"},
		{"ラーメン屋", "らーめんや", "屋", "や"},
		{"ﾗｰﾒﾝ屋", "らーめんや", "屋", "や"},
		{"屋ーメン", "やーめん", "屋", "や"},
		{"テレビ", "てれび", "", ""},
		{"ねこ", "ねこ", "", ""},
		{"猫", "", "", ""},
	} {
		from, to, ruby := furigana(c.word, c.reading)
		if ruby != c.ruby || c.word[from:to] != c.over {
			t.Errorf("furigana(%q, %q) = %q over %q; want %q over %q", c.word, c.reading, ruby, c.word[from:to], c.ruby, c.over)
		}
	}
}

// TestJapaneseLine checks a Japanese line with the larger dictionary: each word in Han characters gets its reading, a verb is found in the list and the vocabulary by its dictionary form, and the punctuation is not marked.
func TestJapaneseLine(t *testing.T) {
	line := "猫が食べた。"
	s := Sources{Spoken: "ja", Readings: true, Analyze: true, List: list(t, "猫\tn5\n食べる\tn3\n"), ListLanguage: "ja", Known: map[string]bool{"食べる": true}}
	got := Lines([]string{line, ""}, s)
	want := []Mark{
		at(line, "猫", Mark{Tone: plugins.ToneSuccess}),
		at(line, "猫", Mark{Ruby: "ねこ"}),
		at(line, "食べた", Mark{Tone: plugins.ToneAccent, Underline: true}),
		at(line, "食", Mark{Ruby: "た"}),
	}
	if len(got) != 2 || !slices.Equal(got[0], want) || got[1] != nil {
		t.Fatalf("Lines = %+v; want %+v", got, want)
	}
	s.Readings = false
	if got := Lines([]string{line}, s)[0]; slices.ContainsFunc(got, func(m Mark) bool { return m.Ruby != "" }) {
		t.Fatalf("readings off: %+v", got)
	}
}

// TestJapaneseLineWithoutAnalyze checks a Japanese line while the larger dictionary may not be read: no readings, and words found as written.
func TestJapaneseLineWithoutAnalyze(t *testing.T) {
	line := "猫が食べた。"
	got := Lines([]string{line}, Sources{Spoken: "ja", Readings: true, List: list(t, "猫\tn5\n食べる\tn3\n"), ListLanguage: "ja"})[0]
	if want := []Mark{at(line, "猫", Mark{Tone: plugins.ToneSuccess})}; !slices.Equal(got, want) {
		t.Fatalf("Lines = %+v; want %+v", got, want)
	}
}

// TestKanaLineOfAnotherLanguageWithoutAnalyze checks a line with kana in a film spoken in another language while the larger Japanese dictionary may not be read: it is cut with the smaller one, as segment.Words cuts it, so a verb has no dictionary form to be found by, where segment.Annotated would have read the larger dictionary for its kana.
func TestKanaLineOfAnotherLanguageWithoutAnalyze(t *testing.T) {
	line := "猫が食べた。"
	s := Sources{Spoken: "en", List: list(t, "食べる\tn3\n"), ListLanguage: "en", Known: map[string]bool{"食べる": true}}
	if got := Lines([]string{line}, s)[0]; got != nil {
		t.Fatalf("Lines = %+v; want nothing marked", got)
	}
	s.Analyze = true
	if got := Lines([]string{line}, s)[0]; !slices.Equal(got, []Mark{at(line, "食べた", Mark{Tone: plugins.ToneAccent, Underline: true})}) {
		t.Fatalf("with Analyze, Lines = %+v", got)
	}
}

// TestLinesStop checks that Lines gives up once Stop reports true, before the next line.
func TestLinesStop(t *testing.T) {
	asked := 0
	s := Sources{Spoken: "en", Known: map[string]bool{"river": true}, Stop: func() bool { asked++; return asked > 1 }}
	if got := Lines([]string{"river", "river", "river"}, s); got != nil || asked != 2 {
		t.Fatalf("Lines = %+v after %d asks; want nil after 2", got, asked)
	}
}

// TestSpokenIn checks that a vocabulary word's language matches a film's case and spaces aside, and that another language or none does not.
func TestSpokenIn(t *testing.T) {
	if !SpokenIn("ja", " JA ") || SpokenIn("ja", "en") || SpokenIn("ja", "") {
		t.Fatal("SpokenIn")
	}
}

// fakeDict is a dictionary of words and their readings.
type fakeDict map[string]string

func (fakeDict) ID() string { return "example.cedict" }

func (d fakeDict) LookUp(word, base, reading string) (lookup.Entry, bool, error) {
	r, ok := d[word]
	return lookup.Entry{Word: word, Reading: r}, ok, nil
}

// TestChineseLine checks a Chinese line: a word a dictionary pack has gets its pinyin, the first dictionary that has a reading giving it; a word none has gets none; and a word list of another language colors nothing.
func TestChineseLine(t *testing.T) {
	line := "我们学习中文"
	s := Sources{Spoken: "zh", Readings: true, Dicts: []lookup.Dictionary{fakeDict{"学习": ""}, fakeDict{"学习": "xué xí", "我们": "wǒ men"}}, List: list(t, "学习\tn5\n"), ListLanguage: "ja"}
	got := Lines([]string{line}, s)[0]
	want := []Mark{at(line, "我们", Mark{Ruby: "wǒ men"}), at(line, "学习", Mark{Ruby: "xué xí"})}
	if !slices.Equal(got, want) {
		t.Fatalf("Lines = %+v; want %+v", got, want)
	}
}

// TestEnglishLine checks a line of a language written with spaces: its words are found in the list in any case, and in the vocabulary, and have no readings.
func TestEnglishLine(t *testing.T) {
	line := "The River flows."
	got := Lines([]string{line}, Sources{Spoken: "en", Readings: true, List: list(t, "river\tn3\n"), ListLanguage: "en", Known: map[string]bool{"flows": true}})[0]
	want := []Mark{at(line, "River", Mark{Tone: plugins.ToneAccent}), at(line, "flows", Mark{Underline: true})}
	if !slices.Equal(got, want) {
		t.Fatalf("Lines = %+v; want %+v", got, want)
	}
}
