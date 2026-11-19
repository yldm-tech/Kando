// Package annotate marks the words of a film's lines for the player to draw over its subtitles: the reading of the words written in Han characters, as furigana over Japanese (from segment.Annotated) and pinyin over Chinese (from the dictionary packs turned on), the tone of each word's tier in the word list chosen (plugins/wordlist), and an underline under the words already in the learner's vocabulary. It imports no interface package; the player's leaf package film/subtitle declares the same Mark, which the app converts these into, so the player imports no plugin package. docs/plugin-system.md (E2) is the design.
package annotate

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"

	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins/wordlist"
	"kando/packages/core"
	"kando/packages/core/segment"
)

// Mark is a run of a line the player draws marked, by byte offsets in the line, Start before End. A mark with Ruby gives the reading drawn over its characters; one without gives their Tone (plugins.ToneAccent, ToneWarn, ToneSuccess or ToneMuted, which the player maps to its theme's roles; "" for none) and whether they are underlined, as a word in the vocabulary is. A word may have both, the ruby mark over its Han characters alone. Keep it the same as film/subtitle.Mark, which the app converts it into.
type Mark struct {
	Start, End int
	Ruby       string
	Tone       string
	Underline  bool
}

// Sources is what lines are marked from, and when to give up. Spoken is the language the film is spoken in (whisper's code or a language tag; "" when not known), which picks how each line is cut and read, else its script does (lookup.WordLanguage). Readings asks for the readings of words in Han characters: a Japanese word's is its reading in the IPA dictionary, which only Analyze lets Lines read (segment.Annotated reads its larger dictionary the first time), a Chinese word's the first of Dicts, in order, that has it. List is the word list whose tiers color the words of the lines of its Language, nil for none; Known the words of the vocabulary, as wordlist.Key writes them, which are underlined, nil for none. A Japanese word is found in List and Known by its dictionary form first, when Analyze gives it, then as written. Stop, when not nil, is asked before each line; once it reports true, Lines gives up and returns nil, as when the marks being computed are no longer wanted.
type Sources struct {
	Spoken       string
	Readings     bool
	Analyze      bool
	Dicts        []lookup.Dictionary
	List         *wordlist.List
	ListLanguage string
	Known        map[string]bool
	Stop         func() bool
}

// Lines marks each of lines, which are the originals of a film's cues, index for index: nil for a line with nothing marked, and nil for all once Stop reports true. It reads dictionaries and may wait for the larger Japanese one, so it runs off the UI's goroutine.
func Lines(lines []string, s Sources) [][]Mark {
	out := make([][]Mark, len(lines))
	pinyin := map[string]string{}
	for i, line := range lines {
		if s.Stop != nil && s.Stop() {
			return nil
		}
		out[i] = s.line(line, pinyin)
	}
	return out
}

// line marks line; pinyin keeps the readings the dictionaries gave the Chinese words seen so far, "" for none, so each is looked up once.
func (s Sources) line(line string, pinyin map[string]string) []Mark {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	lang := lookup.WordLanguage(lookup.Query{Spoken: s.Spoken, Word: line, Line: line})
	listed := s.List != nil && lang != "" && lang == core.BaseLanguage(s.ListLanguage)
	var out []Mark
	at := 0
	for _, m := range s.morphs(lang, line) {
		start, end := at, at+len(m.Text)
		at = end
		if !m.Word {
			continue
		}
		tone, known := "", false
		for _, w := range []string{m.Base, m.Text} {
			if w == "" {
				continue
			}
			if listed && tone == "" {
				if t, ok := s.List.Tier(w); ok {
					tone = t.Tone
				}
			}
			known = known || s.Known[wordlist.Key(w)]
		}
		if tone != "" || known {
			out = append(out, Mark{Start: start, End: end, Tone: tone, Underline: known})
		}
		if !s.Readings || !hasHan(m.Text) {
			continue
		}
		switch lang {
		case "ja":
			if from, to, ruby := furigana(m.Text, hiragana(m.Reading)); ruby != "" {
				out = append(out, Mark{Start: start + from, End: start + to, Ruby: ruby})
			}
		case "zh":
			if ruby := s.pinyin(m.Text, pinyin); ruby != "" {
				out = append(out, Mark{Start: start, End: end, Ruby: ruby})
			}
		}
	}
	return out
}

// morphs cuts line, of the language lang, into its words: by segment.Annotated, with a Japanese word's dictionary form and reading, unless Annotated would cut it as Japanese and Analyze does not let it read the larger dictionary, when segment.Words cuts it, once the smaller one is read. Annotated cuts a line as Japanese for lang "ja", and, for any lang but "zh", for its kana, so a line with kana in a film spoken in English would read the larger dictionary too.
func (s Sources) morphs(lang, line string) []segment.Morph {
	if !s.Analyze && (lang == "ja" || lang != "zh" && strings.IndexFunc(line, scriptKana) >= 0) {
		segment.Load(line)
		tokens := segment.Words(line, nil)
		out := make([]segment.Morph, len(tokens))
		for i, t := range tokens {
			out[i] = segment.Morph{Text: t.Text, Word: t.Word}
		}
		return out
	}
	return segment.Annotated(lang, line)
}

// pinyin is the reading of the Chinese word as the first of Dicts that has it gives it, "" when none does; seen keeps the readings already looked up.
func (s Sources) pinyin(word string, seen map[string]string) string {
	if r, ok := seen[word]; ok {
		return r
	}
	r := ""
	for _, d := range s.Dicts {
		if e, ok, err := d.LookUp(word, "", ""); err == nil && ok && strings.TrimSpace(e.Reading) != "" {
			r = strings.TrimSpace(e.Reading)
			break
		}
	}
	seen[word] = r
	return r
}

// furigana is where the reading of word goes and what it is: the run of word from from to to (byte offsets) and the part of reading, in hiragana, it is read as, the kana word and reading begin and end with alike left out, as 食べた read たべた gives 食 read た and ラーメン屋 read らーめんや gives 屋 read や (the long vowel mark and halfwidth katakana count as kana, kanaFold). "" when there is nothing to put over it: no reading, a word read as it is written, or a run left without Han characters.
func furigana(word, reading string) (from, to int, ruby string) {
	if reading == "" || hiragana(word) == reading {
		return 0, 0, ""
	}
	to, rend := len(word), len(reading)
	for to > 0 && rend > 0 {
		w, wn := utf8.DecodeLastRuneInString(word[:to])
		r, rn := utf8.DecodeLastRuneInString(reading[:rend])
		if f, ok := kanaFold(w); !ok || f != r {
			break
		}
		to, rend = to-wn, rend-rn
	}
	rstart := 0
	for from < to && rstart < rend {
		w, wn := utf8.DecodeRuneInString(word[from:])
		r, rn := utf8.DecodeRuneInString(reading[rstart:])
		if f, ok := kanaFold(w); !ok || f != r {
			break
		}
		from, rstart = from+wn, rstart+rn
	}
	if from >= to || rstart >= rend || !hasHan(word[from:to]) {
		return 0, 0, ""
	}
	return from, to, reading[rstart:rend]
}

// hasHan reports whether s has a Han character.
func hasHan(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0
}

// scriptKana reports whether r is of the hiragana or katakana script, as segment tells a Japanese line by.
func scriptKana(r rune) bool { return unicode.In(r, unicode.Hiragana, unicode.Katakana) }

// kanaFold is r as a reading in hiragana writes it, and whether r is kana: a halfwidth katakana or long vowel mark (ｰ) is widened first, then a katakana that has a hiragana is written in it; the long vowel mark (ー), of no script of its own, counts as kana and stays as it is. A halfwidth voiced mark (ﾞ, ﾟ) widens to a mark of its own, which matches no kana of a reading, so a word written with one keeps its reading over more of it.
func kanaFold(r rune) (rune, bool) {
	if r >= 0xFF61 && r <= 0xFF9F {
		if w := width.LookupRune(r).Wide(); w != 0 {
			r = w
		}
	}
	if r == 'ー' {
		return r, true
	}
	if !scriptKana(r) {
		return r, false
	}
	if r >= 'ァ' && r <= 'ヶ' {
		r -= 0x60
	}
	return r, true
}

// hiragana is s with its katakana written in hiragana (kanaFold), as the IPA dictionary's readings (katakana) are shown.
func hiragana(s string) string {
	return strings.Map(func(r rune) rune {
		f, _ := kanaFold(r)
		return f
	}, strings.TrimSpace(s))
}

// SpokenIn reports whether a word of the vocabulary spoken in word is of a film spoken in film, as Known is taken from the vocabulary: the two languages alike, case and spaces aside.
func SpokenIn(word, film string) bool {
	return strings.EqualFold(strings.TrimSpace(word), strings.TrimSpace(film))
}
