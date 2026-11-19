// Package segment cuts the lines of a film into the words a tap or a click looks up, as core.WordTokens does, and also cuts the runs of the scripts written without spaces between words that it has a dictionary for: Japanese with kagome and its IPA dictionary, Chinese, simplified and traditional, with gse and its dictionary. Both are pure Go and come with the app, so it works offline. A dictionary is read the first time a line needs it, once, on a goroutine of its own, as reading it takes a moment and memory; until then such a run is left out of the words, as core.WordTokens leaves it. Other scripts written without spaces, as Thai, stay out of the words. The desktop's picture and the phone's line card use it alike, so a line is cut the same way on both.
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
	var out []core.WordToken
	add := func(text string, word bool) {
		if text == "" {
			return
		}
		if n := len(out); n > 0 && !out[n-1].Word && !word {
			out[n-1].Text += text
			return
		}
		out = append(out, core.WordToken{Text: text, Word: word})
	}
	for _, t := range tokens {
		if t.Word {
			add(t.Text, true)
			continue
		}
		rest := t.Text
		for rest != "" {
			i := strings.IndexFunc(rest, cjk)
			if i < 0 {
				add(rest, false)
				break
			}
			add(rest[:i], false)
			rest = rest[i:]
			j := strings.IndexFunc(rest, func(r rune) bool { return !cjk(r) })
			if j < 0 {
				j = len(rest)
			}
			run := rest[:j]
			rest = rest[j:]
			words := cut(run)
			if strings.Join(words, "") != run {
				// A cut that is not the run again is not trusted: the run stays out of the words.
				add(run, false)
				continue
			}
			for _, w := range words {
				add(w, strings.IndexFunc(w, unicode.IsLetter) >= 0)
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

// japanese cuts Japanese with kagome and the IPA dictionary, the dictionary without the features of its words (ipa.DictShrink), as only where words end and their parts of speech are used, which takes less memory. An auxiliary verb goes with the verb, adjective or auxiliary before it, so 行った and 食べられない are a word each, as a learner would look them up.
var japanese = newDictionary(func() func(run string) []string {
	t, err := tokenizer.New(ipa.DictShrink(), tokenizer.OmitBosEos())
	if err != nil {
		return nil
	}
	return func(run string) []string {
		var words []string
		last := ""
		for _, tok := range t.Tokenize(run) {
			if tok.Surface == "" {
				continue
			}
			pos := ""
			if p := tok.POS(); len(p) > 0 {
				pos = p[0]
			}
			if n := len(words); n > 0 && pos == "助動詞" && (last == "動詞" || last == "形容詞" || last == "助動詞") {
				words[n-1] += tok.Surface
			} else {
				words = append(words, tok.Surface)
			}
			last = pos
		}
		return words
	}
})

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
