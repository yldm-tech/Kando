package learning

import (
	"strings"
	"time"
	"unicode"

	"kando/packages/sync"
)

// Clip is the sound of a line to dictate: the video it is heard in, kept on the phone, and the span of it to play; Line is what is said there, and Translation its translation, which shows once the learner checked what they typed.
type Clip struct {
	Video             string
	From, To          time.Duration
	Line, Translation string
}

// clipPad is how much of the film before and after a line its clip keeps, as the computer's export cuts a line's sound, so the first and the last word are not cut off where the subtitles' times are a little early or late.
const clipPad = 250 * time.Millisecond

// clipNoEnd is how long the clip of a line whose end is not known plays, as the computer's export plays one.
const clipNoEnd = 4 * time.Second

// ClipOf is the clip of it, heard in video, the film it was saved or looked up in kept on the phone, whose subtitles the player moves by delay against its picture: a saved line's own, or the line a word was looked up in. A line whose end the record does not say, as a word added before ends were kept, ends where the film's subtitle that starts there ends, and one whose end is not known either way plays for clipNoEnd. False for a word with no line, or a line with nothing to say.
func ClipOf(it Item, video string, delay time.Duration, cues []sync.Cue) (Clip, bool) {
	c := Clip{Video: video, Line: it.Text, Translation: it.Meaning}
	end := it.End
	if it.Kind == Word {
		c.Line, c.Translation = it.Context, it.ContextTranslation
	}
	if strings.TrimSpace(c.Line) == "" || video == "" {
		return Clip{}, false
	}
	if end <= it.Start {
		end = 0
		for _, cue := range cues {
			if start := time.Duration(cue.StartMS) * time.Millisecond; start-it.Start < 50*time.Millisecond && it.Start-start < 50*time.Millisecond {
				end = time.Duration(cue.EndMS) * time.Millisecond
				break
			}
		}
	}
	if end <= it.Start {
		end = it.Start + clipNoEnd
	}
	c.From, c.To = max(it.Start+delay-clipPad, 0), end+delay+clipPad
	return c, true
}

// Heard is a word of a line dictated, as the check marks it: its text as the line has it, and whether the learner typed it; a word that is only punctuation is not Counted.
type Heard struct {
	Text           string
	Typed, Counted bool
}

// Dictate checks typed, what the learner heard, against line, what was said: each of line's words is marked typed when typed has it, each of typed's words standing for one of line's at most, whatever their order, case and punctuation. A line written without spaces between its words, in Chinese, Japanese or Thai, is checked a character at a time. It returns the line's words in order, and how many of those counted were typed out of how many.
func Dictate(line, typed string) (words []Heard, hit, total int) {
	split := strings.Fields
	if unspaced(line) {
		split = func(s string) []string {
			var out []string
			for _, r := range s {
				if !unicode.IsSpace(r) {
					out = append(out, string(r))
				}
			}
			return out
		}
	}
	have := map[string]int{}
	for _, w := range split(typed) {
		if n := normal(w); n != "" {
			have[n]++
		}
	}
	for _, w := range split(line) {
		h := Heard{Text: w}
		if n := normal(w); n != "" {
			h.Counted = true
			total++
			if have[n] > 0 {
				have[n]--
				h.Typed = true
				hit++
			}
		}
		words = append(words, h)
	}
	return words, hit, total
}

// ByCharacter reports whether Dictate checks line a character at a time, as a line written without spaces between its words, in Chinese, Japanese or Thai: its score then counts characters rather than words.
func ByCharacter(line string) bool { return unspaced(line) }

// unspaced reports whether line is written without spaces between its words: it has Han, kana or Thai letters.
func unspaced(line string) bool {
	for _, r := range line {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai) {
			return true
		}
	}
	return false
}

// normal is w as the check compares it: its letters, digits and the apostrophes and hyphens inside a word, in lower case, the curly apostrophe made straight.
func normal(w string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(w) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r):
			b.WriteRune(r)
		case r == '’':
			b.WriteRune('\'')
		case r == '\'' || r == '-':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "'-")
}
