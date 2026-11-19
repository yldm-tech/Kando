// Package segment cuts the lines of a film into the words a tap or a click looks up, as core.WordTokens does, and also cuts the runs of the scripts written without spaces between words that it has a dictionary for: Japanese with kagome and its IPA dictionary, Chinese, simplified and traditional, with gse and its dictionary. Both are pure Go and come with the app, so it works offline. A dictionary is read the first time a line needs it, once, on a goroutine of its own, as reading it takes a moment and memory; until then such a run is left out of the words, as core.WordTokens leaves it. Other scripts written without spaces, as Thai, stay out of the words. The desktop's picture and the phone's line card use it alike, so a line is cut the same way on both. Annotated cuts a line the same way and adds each Japanese word's dictionary form and reading, from a larger dictionary only the desktop reads, for the dictionary packs it looks words up in.
package segment

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"strings"
	"sync"
	"unicode"

	"kando/packages/core"

	"github.com/go-ego/gse"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// Words cuts line into its words and what lies between them, which joined again are the line: core.WordTokens, with each run of Japanese or Chinese cut into its words by its dictionary. A line with kana in it is Japanese; one with Han characters and no kana is Chinese.
//
// While the dictionary line needs is not read yet, Words starts reading it in the background and returns the line cut as core.WordTokens cuts it, its Japanese or Chinese out of the words; loaded, when not nil, is then called once the dictionary is read, on the goroutine that read it, so the caller can cut the line again. Words is safe to call from any goroutine, the UI's among them, as it never waits for a dictionary.
func Words(line string, loaded func()) []core.WordToken {
	tokens := core.WordTokens(line)
	d := dictionaryFor(line)
	if d == nil {
		return tokens
	}
	cut := d.cutter(loaded)
	if cut == nil {
		return tokens
	}
	return cutRuns(tokens, cut)
}

// Morph is a run of a line as Annotated cuts it: a word a click looks up (Word) or what lies between words, which joined again are the line; and, for a Japanese word, its dictionary form (Base, 行く for 行った) and its reading in katakana as the IPA dictionary gives it (Reading, イッタ), each "" when not known. Chinese words have neither: their readings come from dictionary packs alone.
type Morph struct {
	Text    string
	Word    bool
	Base    string
	Reading string
}

// Annotated cuts line, of the language lang (whisper's code or a language tag; "" when not known), into its words and what lies between them, as Words does, with the dictionary form and reading of each Japanese word. A line of Japanese is cut with the IPA dictionary with its features (ipa.Dict), which Words leaves out to save memory: it is read the first time Annotated needs it, about 90 MiB more than Words' (docs/plugin-system.md, section 15), so only the desktop calls Annotated, and only for a Japanese dictionary or word list the user turned on. lang "ja" cuts the line as Japanese even with no kana in it, and "zh" as Chinese; another language, or none, goes by its script, as Words does. Unlike Words, Annotated waits for the dictionary it needs, so it is called off the UI's goroutine.
func Annotated(lang, line string) []Morph {
	tokens := core.WordTokens(line)
	d := dictionaryFor(line)
	switch core.BaseLanguage(lang) {
	case "ja":
		d = japanese
	case "zh":
		d = chinese
	}
	switch d {
	case nil:
	case japanese:
		if t := fullJapanese(); t != nil {
			return cutMorphs(tokens, func(run string) []Morph { return japaneseMorphs(t, run) })
		}
	default:
		d.wait()
		cut := d.cutter(nil)
		return cutMorphs(tokens, func(run string) []Morph {
			var out []Morph
			for _, w := range cut(run) {
				out = append(out, Morph{Text: w})
			}
			return out
		})
	}
	out := make([]Morph, len(tokens))
	for i, t := range tokens {
		out[i] = Morph{Text: t.Text, Word: t.Word}
	}
	return out
}

// fullJapanese is the tokenizer of the IPA dictionary with its features, read the first time Annotated cuts Japanese; nil when it cannot be.
var fullJapanese = sync.OnceValue(func() *tokenizer.Tokenizer {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil
	}
	return t
})

// Load reads the dictionary line needs, if any, and waits until it is read, for code off the UI's goroutine that has to cut line now, as tests do.
func Load(line string) {
	if d := dictionaryFor(line); d != nil {
		d.wait()
	}
}

// dictionaryFor is the dictionary that cuts the Japanese or Chinese of line, nil for a line with neither.
func dictionaryFor(line string) *dictionary {
	han := false
	for _, r := range line {
		switch {
		case kana(r):
			return japanese
		case unicode.Is(unicode.Han, r):
			han = true
		}
	}
	if han {
		return chinese
	}
	return nil
}

// kana reports whether r is a letter of the Japanese syllabaries.
func kana(r rune) bool { return unicode.In(r, unicode.Hiragana, unicode.Katakana) }

// cjk reports whether r is part of a run of Japanese or Chinese a dictionary cuts: a Han character, kana, or the marks written among them, the long vowel mark (ー) and the iteration mark (々), which are of no script of their own.
func cjk(r rune) bool {
	switch r {
	case 'ー', 'ｰ', '々', '〆', 'ヶ':
		return true
	}
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}

// cutRuns cuts each run of Japanese or Chinese in tokens, which core.WordTokens left between the words, with cut, each word cut being a word of its own.
func cutRuns(tokens []core.WordToken, cut func(run string) []string) []core.WordToken {
	morphs := cutMorphs(tokens, func(run string) []Morph {
		words := cut(run)
		out := make([]Morph, len(words))
		for i, w := range words {
			out[i] = Morph{Text: w}
		}
		return out
	})
	out := make([]core.WordToken, len(morphs))
	for i, m := range morphs {
		out[i] = core.WordToken{Text: m.Text, Word: m.Word}
	}
	return out
}

// cutMorphs cuts each run of Japanese or Chinese in tokens, which core.WordTokens left between the words, with cut, each morph cut being a word of its own when it has a letter; a cut whose morphs do not join into the run again is not trusted, and the run stays out of the words. What lies between words is joined into one morph.
func cutMorphs(tokens []core.WordToken, cut func(run string) []Morph) []Morph {
	var out []Morph
	add := func(m Morph) {
		if m.Text == "" {
			return
		}
		if n := len(out); n > 0 && !out[n-1].Word && !m.Word {
			out[n-1].Text += m.Text
			return
		}
		out = append(out, m)
	}
	for _, t := range tokens {
		if t.Word {
			add(Morph{Text: t.Text, Word: true})
			continue
		}
		rest := t.Text
		for rest != "" {
			i := strings.IndexFunc(rest, cjk)
			if i < 0 {
				add(Morph{Text: rest})
				break
			}
			add(Morph{Text: rest[:i]})
			rest = rest[i:]
			j := strings.IndexFunc(rest, func(r rune) bool { return !cjk(r) })
			if j < 0 {
				j = len(rest)
			}
			run := rest[:j]
			rest = rest[j:]
			morphs := cut(run)
			var b strings.Builder
			for _, m := range morphs {
				b.WriteString(m.Text)
			}
			if b.String() != run {
				// A cut that is not the run again is not trusted: the run stays out of the words.
				add(Morph{Text: run})
				continue
			}
			for _, m := range morphs {
				m.Word = strings.IndexFunc(m.Text, unicode.IsLetter) >= 0
				if !m.Word {
					m.Base, m.Reading = "", ""
				}
				add(m)
			}
		}
	}
	return out
}

// dictionary is a dictionary read once, the first time a line needs it, on a goroutine of its own: load reads it and returns what cuts a run with it.
type dictionary struct {
	load func() func(run string) []string

	once sync.Once
	done chan struct{}

	mu      sync.Mutex
	cut     func(run string) []string
	waiting []func()
}

func newDictionary(load func() func(run string) []string) *dictionary {
	return &dictionary{load: load, done: make(chan struct{})}
}

// cutter is what cuts a run with the dictionary, nil while it is not read yet, in which case reading it starts, if it has not, and loaded, when not nil, is called once it is read.
func (d *dictionary) cutter(loaded func()) func(run string) []string {
	d.mu.Lock()
	cut := d.cut
	if cut == nil && loaded != nil {
		d.waiting = append(d.waiting, loaded)
	}
	d.mu.Unlock()
	if cut == nil {
		d.start()
	}
	return cut
}

// start reads the dictionary on a goroutine of its own, the first time it is called.
func (d *dictionary) start() {
	d.once.Do(func() { go d.read() })
}

// read reads the dictionary, then tells those waiting for it.
func (d *dictionary) read() {
	cut := d.load()
	if cut == nil {
		// A dictionary that could not be read cuts nothing: its runs stay out of the words.
		cut = func(string) []string { return nil }
	}
	d.mu.Lock()
	d.cut = cut
	waiting := d.waiting
	d.waiting = nil
	close(d.done)
	d.mu.Unlock()
	for _, fn := range waiting {
		fn()
	}
}

// wait reads the dictionary, if it is not read yet, and waits until it is.
func (d *dictionary) wait() {
	d.start()
	<-d.done
}

// japanese cuts Japanese with kagome and the IPA dictionary, the dictionary without the features of its words (ipa.DictShrink), as only where words end and their parts of speech are used, which takes less memory. An auxiliary verb goes with the verb, adjective or auxiliary before it, so 行った and 食べられない are a word each, as a learner would look them up (japaneseMorphs).
var japanese = newDictionary(func() func(run string) []string {
	t, err := tokenizer.New(ipa.DictShrink(), tokenizer.OmitBosEos())
	if err != nil {
		return nil
	}
	return func(run string) []string {
		var words []string
		for _, m := range japaneseMorphs(t, run) {
			words = append(words, m.Text)
		}
		return words
	}
})

// japaneseMorphs cuts the Japanese run with t into words, an auxiliary verb going with the verb, adjective or auxiliary before it. A word's Base is the dictionary form of its first token (行く of 行った) and its Reading its tokens' readings joined, both "" when t's dictionary has no features (ipa.DictShrink) or a token has none.
func japaneseMorphs(t *tokenizer.Tokenizer, run string) []Morph {
	var words []Morph
	last := ""
	for _, tok := range t.Tokenize(run) {
		if tok.Surface == "" {
			continue
		}
		pos := ""
		if p := tok.POS(); len(p) > 0 {
			pos = p[0]
		}
		reading := feature(tok.Reading())
		if n := len(words); n > 0 && pos == "助動詞" && (last == "動詞" || last == "形容詞" || last == "助動詞") {
			w := &words[n-1]
			w.Text += tok.Surface
			if w.Reading != "" && reading != "" {
				w.Reading += reading
			} else {
				w.Reading = ""
			}
		} else {
			words = append(words, Morph{Text: tok.Surface, Base: feature(tok.BaseForm()), Reading: reading})
		}
		last = pos
	}
	return words
}

// feature is a feature of a token, "" when the dictionary has none for it ("*").
func feature(s string, ok bool) string {
	if !ok || s == "*" {
		return ""
	}
	return s
}

// chinese cuts Chinese, simplified and traditional, with gse and its dictionaries of both (dict/zh_s.txt.gz, dict/zh_t.txt.gz), guessing at words they lack (its HMM). It reads these compressed copies rather than calling gse.LoadDictEmbed, which would keep every dictionary gse embeds in the apps, its Japanese one and a word-weight list among them, about 38 MB; with LoadDictEmbed never called the linker leaves those out, and these two are about 3 MB.
var chinese = newDictionary(func() func(run string) []string {
	simplified, err := gunzip(zhS)
	if err != nil {
		return nil
	}
	traditional, err := gunzip(zhT)
	if err != nil {
		return nil
	}
	seg := &gse.Segmenter{SkipLog: true}
	if err := seg.LoadDictStr(simplified + "\n" + traditional); err != nil {
		return nil
	}
	return func(run string) []string { return seg.Cut(run, true) }
})

// The dictionaries of gse v1.3.1 (data/dict/zh/s_1.txt and t_1.txt), gzipped, under gse's licence (dict/LICENSE).
var (
	//go:embed dict/zh_s.txt.gz
	zhS []byte
	//go:embed dict/zh_t.txt.gz
	zhT []byte
)

// gunzip is the text gzipped in b.
func gunzip(b []byte) (string, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	text, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(text), nil
}
