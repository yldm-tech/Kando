package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"kando/packages/core"
)

// Proofreader is a Pipeline that corrects what its speech recognition misheard in a transcript before it is translated, as Tools does through its translator. A pipeline that is not one keeps the lines as they were heard.
type Proofreader interface {
	// Proofread returns segments with the words the speech recognition misheard corrected, title naming the video for context. It changes words only in place (ApplyFixes), never the timing or the number of lines; a batch it could not proofread keeps its lines as they were.
	Proofread(ctx context.Context, segments []core.Segment, title string) ([]core.Segment, error)
}

// proofreadBatch bounds how many lines go into one proofreading request. Replies are mostly empty strings, one for each line that needs no correction, so a batch can be twice a translation's, which gives the model more of the film to judge a word by.
const proofreadBatch = 80

// proofreadPrompt is the instruction a proofreading request is sent with; {{title}} becomes the video's title. The lines are a speech recognizer's, so they hold its mishearings, and the reply is checked word by word (ApplyFixes): anything but a word put in place of another is dropped.
const proofreadPrompt = `You proofread the transcript of a video titled "{{title}}". A speech recognizer wrote the lines down, and it mishears words: words that sound alike (in Chinese and Japanese often a character for another that sounds the same), names, technical terms of the video's subject, letters and numbers.
- Read every line. Correct only words the recognizer misheard, using the title and the other lines to tell what was said. A misheard term usually recurs: correct it in every line it is in.
- Do not rephrase, add, remove or reorder words, do not change punctuation, and do not translate. Keep each line in its language and script.

The user sends a JSON array of the lines, each {"id": its number, "text": the line}. Reply with only a JSON array with one element for each line you corrected, and none for the others: {"id": the line's number, "heard": the line exactly as it was sent, "text": the corrected line}. Reply [] when no line needs a correction.`

// Proofread corrects the words a speech recognizer misheard in segments, in batches of proofreadBatch lines, through the same endpoint and model translations use. A correction is kept only where it puts words in place of others (ApplyFixes), and one made in two lines or more is made wherever else the film has the same words (spreadFixes), as a model corrects a recurring term in most of the lines it is in but not all. A batch whose reply cannot be used after it was sent again keeps its lines as they were heard, and the rest of the film is still proofread; a key, model or address the API rejects, or an account out of credit, fails the proofreading, which Run then skips.
func (t ChatTranslator) Proofread(ctx context.Context, segments []core.Segment, title string) ([]core.Segment, error) {
	if err := t.offline(); err != nil {
		return nil, err
	}
	endpoint := Endpoint{BaseURL: t.BaseURL, APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return nil, err
		}
	}
	system := strings.ReplaceAll(proofreadPrompt, "{{title}}", strings.TrimSpace(title))
	renewed := false
	out := make([]core.Segment, len(segments))
	copy(out, segments)
	batches := (len(segments) + proofreadBatch - 1) / proofreadBatch
	for start := 0; start < len(segments); start += proofreadBatch {
		batch := segments[start:min(start+proofreadBatch, len(segments))]
		progress := core.Fraction(float64(start) / float64(len(segments)))
		progress.Proofreading = true
		progress.Batch, progress.Batches, progress.BatchSize = start/proofreadBatch+1, batches, proofreadBatch
		progress.Lines, progress.Of = start, len(segments)
		reportProgress(ctx, progress)
		texts := make([]string, len(batch))
		for i, s := range batch {
			texts[i] = s.Text
		}
		fixes, err := t.proofreadTexts(ctx, endpoint, system, texts, progress)
		var status *statusError
		if t.Credential != nil && !renewed && errors.As(err, &status) && status.code == http.StatusUnauthorized {
			renewed = true
			if endpoint, err = t.Credential(ctx, true); err != nil {
				return nil, err
			}
			fixes, err = t.proofreadTexts(ctx, endpoint, system, texts, progress)
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if !skippableBatch(err) {
				return nil, err
			}
			continue
		}
		for i, fix := range fixes {
			out[start+i].Text = ApplyFixes(texts[i], fix)
		}
	}
	spreadFixes(segments, out)
	for i := range out {
		out[i].Text = NormalizePunctuation(out[i].Text)
	}
	done := core.Fraction(1)
	done.Proofreading = true
	reportProgress(ctx, done)
	return out, nil
}

// proofreadFix is a correction a proofreading reply suggests: the line numbered ID, which the model says it heard as Heard, written as Text.
type proofreadFix struct {
	ID    int    `json:"id"`
	Heard string `json:"heard"`
	Text  string `json:"text"`
}

// proofreadTexts sends one batch of lines to proofread, again while the reply is not a JSON array of corrections, up to maxBatchSends times, and returns each line's correction, "" for a line with none. A correction whose Heard is not the line its number names, as a model that lost count of the lines writes, is dropped: put on the wrong line it would read as a plausible correction of it.
func (t ChatTranslator) proofreadTexts(ctx context.Context, endpoint Endpoint, system string, texts []string, progress core.Progress) ([]string, error) {
	type numbered struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
	}
	lines := make([]numbered, len(texts))
	for i, text := range texts {
		lines[i] = numbered{ID: i + 1, Text: text}
	}
	input, err := json.Marshal(lines)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(chatRequest{
		Model:    t.Model,
		Messages: []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: string(input)}},
	})
	if err != nil {
		return nil, err
	}
	for send := 1; ; send++ {
		reply, err := t.complete(ctx, endpoint, body, progress)
		if err != nil {
			return nil, err
		}
		var fixes []proofreadFix
		if len(reply.Choices) == 0 {
			err = errors.New("proofreading API returned no choices")
		} else if err = json.Unmarshal([]byte(stripCodeFence(reply.Choices[0].Message.Content)), &fixes); err != nil {
			err = fmt.Errorf("proofreading is not a JSON array of corrections: %w", err)
		}
		if err == nil {
			out := make([]string, len(texts))
			for _, fix := range fixes {
				if i := fix.ID - 1; i >= 0 && i < len(texts) && sameWords(fix.Heard, texts[i]) {
					out[i] = fix.Text
				}
			}
			return out, nil
		}
		if send == maxBatchSends || ctx.Err() != nil {
			return nil, err
		}
	}
}

// Proofreads reports whether now is heard as proofreading leaves it: the same words, but for those ApplyFixes takes in place of others, whatever the punctuation, so a line saved from a film before it was proofread can be told from a line of another film made again over it.
func Proofreads(heard, now string) bool { return sameWords(ApplyFixes(heard, now), now) }

// sameWords reports whether a and b are the same words, whatever their punctuation and spacing (fixWords).
func sameWords(a, b string) bool {
	x, y := fixWords(a), fixWords(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i].text != y[i].text {
			return false
		}
	}
	return true
}

// maxFixRun is the most words, or Chinese, Japanese, Thai, Lao, Khmer and Myanmar characters, one correction may put in place of as many others in a row: a misheard word is a character or two, or a name; a longer run is a rewording.
const maxFixRun = 2

// ApplyFixes is line with the corrections fixed suggests for it put in place, and nothing else: fixed is compared with line word by word (fixWords), each Chinese, Japanese, Thai, Lao, Khmer or Myanmar character counting as a word, and only a run of at most maxFixRun words put in place of as many words of line is taken, in place, keeping line's punctuation and spacing. Words fixed adds or leaves out are not, so a proofreader cannot reword a line, only correct what was misheard in it. An empty fixed, a proofreader's "no correction", returns line with its punctuation normalized (NormalizePunctuation).
func ApplyFixes(line, fixed string) string {
	fixed = strings.TrimSpace(strings.ReplaceAll(fixed, "\n", " "))
	if fixed == "" || fixed == line {
		return NormalizePunctuation(line)
	}
	have, want := fixWords(line), fixWords(fixed)
	// lcs[i][j] is the longest run of words have[i:] and want[j:] share in order.
	lcs := make([][]int, len(have)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(want)+1)
	}
	for i := len(have) - 1; i >= 0; i-- {
		for j := len(want) - 1; j >= 0; j-- {
			if have[i].text == want[j].text {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	// replace maps a word of line, by its index, to the word fixed puts in its place.
	replace := map[int]string{}
	i, j := 0, 0
	for i < len(have) || j < len(want) {
		if i < len(have) && j < len(want) && have[i].text == want[j].text {
			i, j = i+1, j+1
			continue
		}
		// A gap between two shared words: the words line has there and those fixed has.
		gi, gj := i, j
		for i < len(have) || j < len(want) {
			if i < len(have) && j < len(want) && have[i].text == want[j].text {
				break
			}
			if j >= len(want) || (i < len(have) && lcs[i+1][j] >= lcs[i][j+1]) {
				i++
			} else {
				j++
			}
		}
		if n := i - gi; n > 0 && n == j-gj && n <= maxFixRun {
			for k := 0; k < n; k++ {
				replace[gi+k] = want[gj+k].text
			}
		}
	}
	if len(replace) == 0 {
		return NormalizePunctuation(line)
	}
	return NormalizePunctuation(replaceWords(line, have, replace))
}

// spreadFixes makes in fixed, the proofread lines of heard, the corrections made in at least two lines wherever else the same words are left: each correction is the words put in place together with the word before and the word after them (only the one there is, at the start or end of a line), so "网罗" corrected to "网络" at the end of two lines corrects every other "网罗", "神经网罗是" corrected twice corrects no "张罗是", and "倒一" corrected once to "退一" corrects no "倒一杯水".
func spreadFixes(heard, fixed []core.Segment) {
	type fix struct {
		from, to string
		lines    int
	}
	fixes := map[string]*fix{}
	for i := range heard {
		was, now := fixWords(heard[i].Text), fixWords(fixed[i].Text)
		if len(was) != len(now) {
			continue
		}
		seen := map[string]bool{}
		for a := 0; a < len(was); {
			if was[a].text == now[a].text {
				a++
				continue
			}
			b := a
			for b < len(was) && was[b].text != now[b].text {
				b++
			}
			if a == 0 && b == len(was) {
				a = b
				continue
			}
			from, to := wordsText(was[a:b]), wordsText(now[a:b])
			if b < len(was) {
				from, to = from+"\x00"+was[b].text, to+"\x00"+was[b].text
			}
			if a > 0 {
				from, to = was[a-1].text+"\x00"+from, was[a-1].text+"\x00"+to
			}
			if f, ok := fixes[from]; !ok {
				fixes[from] = &fix{from: from, to: to}
			} else if f.to != to {
				f.lines = -1 << 30
			}
			if !seen[from] {
				seen[from] = true
				fixes[from].lines++
			}
			a = b
		}
	}
	// The corrections are made in the same order every run, as one may overlap another.
	for _, key := range slices.Sorted(maps.Keys(fixes)) {
		f := fixes[key]
		if f.lines < 2 {
			continue
		}
		from, to := strings.Split(f.from, "\x00"), strings.Split(f.to, "\x00")
		for i := range fixed {
			words := fixWords(fixed[i].Text)
			replace := map[int]string{}
			for a := 0; a+len(from) <= len(words); a++ {
				match := true
				for k, w := range from {
					if words[a+k].text != w {
						match = false
						break
					}
				}
				if match {
					for k, w := range to {
						replace[a+k] = w
					}
				}
			}
			if len(replace) > 0 {
				fixed[i].Text = replaceWords(fixed[i].Text, words, replace)
			}
		}
	}
}

// wordsText is words joined with a NUL between them, so a run of two words is not taken for one word of the same letters.
func wordsText(words []fixWord) string {
	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.text
	}
	return strings.Join(texts, "\x00")
}

// replaceWords is line, whose words are words (fixWords), with each word whose index replace has written as replace says, keeping everything between the words.
func replaceWords(line string, words []fixWord, replace map[int]string) string {
	var b strings.Builder
	at := 0
	for k, w := range words {
		b.WriteString(line[at:w.start])
		if text, ok := replace[k]; ok {
			b.WriteString(text)
		} else {
			b.WriteString(w.text)
		}
		at = w.end
	}
	b.WriteString(line[at:])
	return b.String()
}

// fixWord is a word of a line ApplyFixes compares, and where it lies in the line, in bytes.
type fixWord struct {
	text       string
	start, end int
}

// fixWords are the words of line: each Chinese or Japanese character on its own, each Thai, Lao, Khmer or Myanmar character on its own with the vowel and tone marks written on it, and each run of other letters and digits, with their marks and the apostrophes inside a word (team's). Spaces and punctuation are not words.
func fixWords(line string) []fixWord {
	var words []fixWord
	start := -1
	// cluster is whether the word open is a clustered character (clustered), which takes only the marks after it.
	cluster := false
	end := func(at int) {
		if start >= 0 {
			words = append(words, fixWord{text: line[start:at], start: start, end: at})
			start, cluster = -1, false
		}
	}
	for at, r := range line {
		mark := unicode.In(r, unicode.Mn, unicode.Mc)
		switch {
		case cluster && mark:
		case ideographic(r):
			end(at)
			words = append(words, fixWord{text: string(r), start: at, end: at + len(string(r))})
		case clustered(r) && !mark:
			end(at)
			start, cluster = at, true
		case unicode.IsLetter(r) || unicode.IsDigit(r) || mark || ((r == '\'' || r == '’') && start >= 0 && !cluster):
			if cluster {
				end(at)
			}
			if start < 0 {
				start = at
			}
		default:
			end(at)
		}
	}
	end(len(line))
	return words
}

// ideographic reports whether r is written without spaces between words, so each character is a word to ApplyFixes: Chinese characters and Japanese kana.
func ideographic(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}

// clustered reports whether r is of a script written without spaces between words whose characters carry marks, Thai, Lao, Khmer and Myanmar, so each character with its marks is a word to ApplyFixes: taken as a run of letters, a whole phrase would be one word, and maxFixRun would let a proofreader reword it. Its punctuation stays as whisper writes it (wideNeighbour).
func clustered(r rune) bool {
	return unicode.In(r, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar)
}

// fullWidth is the punctuation Chinese and Japanese write full width, by the ASCII mark a speech recognizer writes for it.
var fullWidth = map[rune]rune{',': '，', '?': '？', '!': '！', ':': '：', ';': '；'}

// NormalizePunctuation is line with the ASCII commas, question marks, exclamation marks, colons and semicolons next to a Chinese or Japanese character written full width, as those languages write them: whisper writes "好的,我们开始" for "好的，我们开始", and "第二步是B,然后" for "第二步是B，然后". A mark between letters or digits, as in "A,B,C", stays as it is.
func NormalizePunctuation(line string) string {
	runes := []rune(line)
	for i, r := range runes {
		wide, ok := fullWidth[r]
		if !ok {
			continue
		}
		if (i > 0 && wideNeighbour(runes[i-1])) || (i+1 < len(runes) && wideNeighbour(runes[i+1])) {
			runes[i] = wide
		}
	}
	return string(runes)
}

// wideNeighbour reports whether r, next to an ASCII mark, makes it a Chinese or Japanese one: a Chinese or Japanese character, or their punctuation, including a mark just written full width, so "你好!!" becomes "你好！！" and "「好」," becomes "「好」，".
func wideNeighbour(r rune) bool {
	return ideographic(r) || (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF01 && r <= 0xFF0F) || (r >= 0xFF1A && r <= 0xFF20) || (r >= 0xFF3B && r <= 0xFF40) || (r >= 0xFF5B && r <= 0xFF65)
}
